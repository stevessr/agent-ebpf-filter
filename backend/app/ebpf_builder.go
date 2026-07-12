package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"golang.org/x/sys/unix"
)

// ---- moved from backend/zz_merged_backend.go section ebpf_builder.go ----

// loadedEBPFPlugin tracks an attached user plugin so we can detach on disable/delete.
type loadedEBPFPlugin struct {
	collection *ebpf.Collection
	links      []link.Link
	loadError  string
}

var (
	loadedPluginsMu sync.Mutex
	loadedPlugins   = make(map[string]*loadedEBPFPlugin)
	loadingPlugins  = make(map[string]*ebpfPluginLoadReservation)
)

type ebpfPluginLoadReservation struct {
	canceled bool
}

// File-bearing preprocessor directives and inline assembly are deliberately
// restricted because clang runs against host files on a low-trust API surface.
var (
	userBPFFileDirectivePattern = regexp.MustCompile(`(?m)^\s*#\s*(include|include_next|import|embed)\b([^\r\n]*)`)
	userBPFIncbinPattern        = regexp.MustCompile(`(?i)\.incbin\b`)
	userBPFInlineAsmPattern     = regexp.MustCompile(`(?i)\b(?:__asm__|__asm|asm)\s*(?:volatile\s*)?\(`)
)

const (
	maxUserBPFSourceBytes            = 256 << 10
	maxUserBPFObjectBytes      int64 = 32 << 20
	maxUserBPFDiagnosticsBytes       = 1 << 20
	userBPFCompileTimeout            = 15 * time.Second
)

var userBPFCompileSlots = make(chan struct{}, 2)

type cappedCompilerOutput struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	remaining int
	truncated bool
}

func (w *cappedCompilerOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	if n > w.remaining {
		w.truncated = true
	}
	if w.remaining > 0 {
		keep := n
		if keep > w.remaining {
			keep = w.remaining
		}
		_, _ = w.buf.Write(p[:keep])
		w.remaining -= keep
	}
	return n, nil
}

func (w *cappedCompilerOutput) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := append([]byte(nil), w.buf.Bytes()...)
	if w.truncated {
		out = append(out, []byte("\n[compiler output truncated]\n")...)
	}
	return out
}

// validateUserBPFSource performs lightweight checks before we hand the source
// off to clang. We are not a sandbox — but rejecting obvious abuse keeps the
// resulting object file deterministic and small.
func validateUserBPFSource(source string) error {
	if strings.TrimSpace(source) == "" {
		return errors.New("source is empty")
	}
	if len(source) > maxUserBPFSourceBytes {
		return errors.New("source exceeds 256 KiB")
	}
	if strings.IndexByte(source, 0) >= 0 {
		return errors.New("source contains a NUL byte")
	}
	preprocessed := strings.ReplaceAll(strings.ReplaceAll(source, "\\\r\n", ""), "\\\n", "")
	preprocessed = stripUserBPFComments(preprocessed)
	if strings.Contains(preprocessed, "??=") || strings.Contains(preprocessed, "%:") {
		return errors.New("source contains disallowed alternate preprocessor token")
	}
	if strings.Contains(preprocessed, "__has_include") {
		return errors.New("source contains disallowed include probe")
	}
	for _, match := range userBPFFileDirectivePattern.FindAllStringSubmatch(preprocessed, -1) {
		directive := match[1]
		operand := strings.TrimSpace(match[2])
		if directive != "include" || !allowedUserBPFInclude(operand) {
			return fmt.Errorf("source contains disallowed #%s directive %q", directive, operand)
		}
	}
	if userBPFIncbinPattern.MatchString(preprocessed) || userBPFInlineAsmPattern.MatchString(preprocessed) {
		return errors.New("source contains disallowed inline assembler")
	}
	if !strings.Contains(source, "SEC(") {
		return errors.New("source must declare at least one SEC(\"...\") program")
	}
	return nil
}

func stripUserBPFComments(source string) string {
	const (
		commentNormal = iota
		commentLine
		commentBlock
		commentString
		commentChar
	)
	state := commentNormal
	out := []byte(source)
	for i := 0; i < len(out); i++ {
		switch state {
		case commentNormal:
			switch {
			case out[i] == '/' && i+1 < len(out) && out[i+1] == '/':
				out[i], out[i+1] = ' ', ' '
				i++
				state = commentLine
			case out[i] == '/' && i+1 < len(out) && out[i+1] == '*':
				out[i], out[i+1] = ' ', ' '
				i++
				state = commentBlock
			case out[i] == '"':
				state = commentString
			case out[i] == '\'':
				state = commentChar
			}
		case commentLine:
			if out[i] == '\n' || out[i] == '\r' {
				state = commentNormal
			} else {
				out[i] = ' '
			}
		case commentBlock:
			if out[i] == '*' && i+1 < len(out) && out[i+1] == '/' {
				out[i], out[i+1] = ' ', ' '
				i++
				state = commentNormal
			} else if out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
			}
		case commentString, commentChar:
			terminator := byte('"')
			if state == commentChar {
				terminator = '\''
			}
			if out[i] == '\\' && i+1 < len(out) {
				i++
			} else if out[i] == terminator {
				state = commentNormal
			}
		}
	}
	return string(out)
}

func allowedUserBPFInclude(operand string) bool {
	if operand == `"vmlinux.h"` {
		return true
	}
	if len(operand) < 3 || operand[0] != '<' || operand[len(operand)-1] != '>' {
		return false
	}
	header := operand[1 : len(operand)-1]
	if strings.Contains(header, "..") || strings.HasPrefix(header, "/") || strings.ContainsAny(header, "\\\x00") {
		return false
	}
	if strings.HasPrefix(header, "bpf/") || strings.HasPrefix(header, "linux/") {
		return strings.HasSuffix(header, ".h")
	}
	return false
}

// clangBinary discovers a clang capable of emitting BPF bytecode.
func clangBinary() (string, error) {
	for _, name := range []string{"clang", "clang-18", "clang-17", "clang-16", "clang-15", "clang-14"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", errors.New("clang not found on PATH (install clang ≥14 to build user eBPF programs)")
}

// vmlinuxIncludeDir returns the directory containing vmlinux.h so user
// programs can `#include "vmlinux.h"`. We reuse the project's bundled copy
// under backend/ebpf when present.
func vmlinuxIncludeDir() string {
	candidates := []string{
		"backend/ebpf",
		"./backend/ebpf",
		"../backend/ebpf",
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "vmlinux.h")); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

// CompileUserBPF compiles the supplied source with clang and writes the BPF
// object next to the source. Returns the path to the resulting .o.
func CompileUserBPF(pluginID, source string) (string, []byte, error) {
	return CompileUserBPFContext(context.Background(), pluginID, source)
}

// CompileUserBPFContext is CompileUserBPF with caller cancellation. A bounded
// timeout is still applied even when the caller has no deadline.
func CompileUserBPFContext(parent context.Context, pluginID, source string) (string, []byte, error) {
	if err := validatePluginID(pluginID); err != nil {
		return "", nil, err
	}
	if err := validateUserBPFSource(source); err != nil {
		return "", nil, err
	}
	clang, err := clangBinary()
	if err != nil {
		return "", nil, err
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, userBPFCompileTimeout)
	defer cancel()
	return compileUserBPFWithContext(ctx, clang, pluginID, source)
}

func compileUserBPFWithContext(ctx context.Context, clang, pluginID, source string) (string, []byte, error) {
	release, err := acquirePluginArtifactLock(ctx, pluginID)
	if err != nil {
		return "", nil, fmt.Errorf("wait for plugin artifact lock: %w", err)
	}
	defer release()
	objectPath := pluginDisplayPath(pluginID, "program.o")
	return compileUserBPFInDir(ctx, clang, pluginID, source, filepath.Dir(objectPath), objectPath)
}

func compileUserBPFInDir(ctx context.Context, clang, pluginID, source, pluginDir, objectPath string) (string, []byte, error) {
	if err := validatePluginID(pluginID); err != nil {
		return "", nil, err
	}
	if err := validateUserBPFSource(source); err != nil {
		return "", nil, err
	}
	select {
	case userBPFCompileSlots <- struct{}{}:
		defer func() { <-userBPFCompileSlots }()
	case <-ctx.Done():
		return "", nil, fmt.Errorf("clang compile queue: %w", ctx.Err())
	}
	dir, err := secureOpenOrCreateDirectory(pluginDir)
	if err != nil {
		return "", nil, fmt.Errorf("open plugin directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Chmod(0o700); err != nil {
		return "", nil, err
	}
	if err := chownArtifactFile(dir); err != nil {
		return "", nil, err
	}
	sourceFile, sourceTemp, err := createRecordingTemp(dir, "source")
	if err != nil {
		return "", nil, err
	}
	sourcePublished := false
	defer func() {
		_ = sourceFile.Close()
		if !sourcePublished {
			_ = unix.Unlinkat(int(dir.Fd()), sourceTemp, 0)
		}
	}()
	if _, err := sourceFile.WriteString(source); err != nil {
		return "", nil, err
	}
	if err := sourceFile.Sync(); err != nil {
		return "", nil, err
	}
	if err := sourceFile.Close(); err != nil {
		return "", nil, err
	}
	sourceInput, err := openRecordingChild(dir, sourceTemp, os.O_RDONLY, 0)
	if err != nil {
		return "", nil, err
	}
	defer sourceInput.Close()
	objectFile, objectTemp, err := createRecordingTemp(dir, "object")
	if err != nil {
		return "", nil, err
	}
	objectPublished := false
	defer func() {
		_ = objectFile.Close()
		if !objectPublished {
			_ = unix.Unlinkat(int(dir.Fd()), objectTemp, 0)
		}
	}()

	args := []string{
		"-O2", "-g", "-Wall",
		"-target", "bpf",
		"-x", "c",
		"-D__TARGET_ARCH_x86",
		"-c", "/proc/self/fd/3",
		"-o", "/proc/self/fd/4",
	}
	if dir := vmlinuxIncludeDir(); dir != "" {
		args = append([]string{"-I", dir}, args...)
	}
	cmd := exec.CommandContext(ctx, clang, args...)
	cmd.ExtraFiles = []*os.File{sourceInput, objectFile}
	output := &cappedCompilerOutput{remaining: maxUserBPFDiagnosticsBytes}
	cmd.Stdout, cmd.Stderr = output, output
	err = cmd.Run()
	out := output.Bytes()
	if err != nil {
		if ctx.Err() != nil {
			return "", out, fmt.Errorf("clang timed out or canceled: %w", ctx.Err())
		}
		return "", out, fmt.Errorf("clang failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if err := validateRecordingRegularFile(objectFile); err != nil {
		return "", out, fmt.Errorf("validate clang object: %w", err)
	}
	info, err := objectFile.Stat()
	if err != nil {
		return "", out, err
	}
	if info.Size() <= 0 || info.Size() > maxUserBPFObjectBytes {
		return "", out, fmt.Errorf("clang object size %d is outside the allowed range", info.Size())
	}
	if err := objectFile.Sync(); err != nil {
		return "", out, err
	}
	if err := rejectUnsafeRecordingDestination(dir, "source.c"); err != nil {
		return "", out, fmt.Errorf("validate published source destination: %w", err)
	}
	if err := rejectUnsafeRecordingDestination(dir, "program.o"); err != nil {
		return "", out, fmt.Errorf("validate published object destination: %w", err)
	}
	// Publish the executable object first. Until RecordCompile advances the
	// manifest checksum, a crash or partial publication therefore fails closed
	// instead of loading an old object under newly displayed source.
	if err := replaceRecordingDestination(dir, objectTemp, "program.o"); err != nil {
		return "", out, err
	}
	objectPublished = true
	if err := replaceRecordingDestination(dir, sourceTemp, "source.c"); err != nil {
		return "", out, err
	}
	sourcePublished = true
	return objectPath, out, nil
}

// LoadEBPFPlugin reads the object file from disk and attaches the requested program.
func LoadEBPFPlugin(m *PluginManifest) error {
	return LoadEBPFPluginContext(context.Background(), m)
}

// LoadEBPFPluginContext propagates request cancellation while waiting for
// artifact access and between the non-cancelable kernel load/attach steps.
func LoadEBPFPluginContext(ctx context.Context, m *PluginManifest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateLoadableEBPFPluginManifest(m); err != nil {
		return err
	}
	if m.ObjectSHA256 == "" {
		return errors.New("compiled object checksum is missing; compile the plugin before loading")
	}
	reservation, err := reserveEBPFPluginLoad(m.ID)
	if err != nil {
		return err
	}
	installed := false
	defer func() {
		if !installed {
			releaseEBPFPluginLoad(m.ID, reservation)
		}
	}()

	releaseArtifacts, err := acquirePluginArtifactLock(ctx, m.ID)
	if err != nil {
		return fmt.Errorf("wait for plugin artifact lock: %w", err)
	}
	object, err := readPluginFile(m.ID, "program.o", maxUserBPFObjectBytes)
	releaseArtifacts()
	if err != nil {
		return fmt.Errorf("plugin object missing or unsafe: %w", err)
	}
	if sha256Hex(object) != m.ObjectSHA256 {
		return errors.New("plugin object checksum does not match manifest")
	}
	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(object))
	if err != nil {
		return fmt.Errorf("load collection spec: %w", err)
	}
	programSpec, err := validateUserBPFCollectionSpec(spec, m)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("plugin load canceled before kernel verification: %w", err)
	}
	// Loading unselected programs needlessly consumes verifier time and kernel
	// memory. Keep only the explicitly requested entry point.
	spec.Programs = map[string]*ebpf.ProgramSpec{m.ProgramName: programSpec}
	coll, err := ebpf.NewCollectionWithOptions(spec, ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{LogDisabled: true},
	})
	if err != nil {
		return fmt.Errorf("instantiate collection: %w", err)
	}
	if err := ctx.Err(); err != nil {
		coll.Close()
		return fmt.Errorf("plugin load canceled after kernel verification: %w", err)
	}
	prog, ok := coll.Programs[m.ProgramName]
	if !ok {
		coll.Close()
		names := make([]string, 0, len(coll.Programs))
		for n := range coll.Programs {
			names = append(names, n)
		}
		return fmt.Errorf("program %q not found in object (available: %s)", m.ProgramName, strings.Join(names, ", "))
	}

	var attached link.Link
	switch m.AttachKind {
	case PluginAttachTracepoint:
		category, name, err := splitTracepointTarget(m.AttachTarget)
		if err != nil {
			coll.Close()
			return err
		}
		attached, err = link.Tracepoint(category, name, prog, nil)
		if err != nil {
			coll.Close()
			return fmt.Errorf("attach tracepoint: %w", err)
		}
	case PluginAttachKprobe:
		attached, err = link.Kprobe(m.AttachTarget, prog, nil)
		if err != nil {
			coll.Close()
			return fmt.Errorf("attach kprobe: %w", err)
		}
	case PluginAttachKretprobe:
		attached, err = link.Kretprobe(m.AttachTarget, prog, nil)
		if err != nil {
			coll.Close()
			return fmt.Errorf("attach kretprobe: %w", err)
		}
	case PluginAttachLSM:
		attached, err = link.AttachLSM(link.LSMOptions{Program: prog})
		if err != nil {
			coll.Close()
			target := strings.TrimSpace(m.AttachTarget)
			if target != "" {
				return fmt.Errorf("attach lsm %s: %w", target, err)
			}
			return fmt.Errorf("attach lsm: %w", err)
		}
	default:
		coll.Close()
		return fmt.Errorf("unsupported attach kind %q", m.AttachKind)
	}
	if err := ctx.Err(); err != nil {
		if attached != nil {
			_ = attached.Close()
		}
		coll.Close()
		return fmt.Errorf("plugin load canceled after attach: %w", err)
	}

	entry := &loadedEBPFPlugin{
		collection: coll,
		links:      []link.Link{attached},
	}
	old, err := installLoadedEBPFPlugin(m.ID, reservation, entry)
	if err != nil {
		closeLoadedPlugin(entry)
		return err
	}
	installed = true
	closeLoadedPlugin(old)
	return nil
}

func reserveEBPFPluginLoad(id string) (*ebpfPluginLoadReservation, error) {
	loadedPluginsMu.Lock()
	defer loadedPluginsMu.Unlock()
	if _, exists := loadingPlugins[id]; exists {
		return nil, fmt.Errorf("plugin %q is already loading", id)
	}
	ids := make(map[string]struct{}, len(loadedPlugins)+len(loadingPlugins))
	for loadedID, entry := range loadedPlugins {
		if loadedEBPFPluginActive(entry) {
			ids[loadedID] = struct{}{}
		}
	}
	for loadingID := range loadingPlugins {
		ids[loadingID] = struct{}{}
	}
	if _, alreadyCounted := ids[id]; !alreadyCounted && len(ids) >= maxLoadedEBPFPlugins {
		return nil, fmt.Errorf("loaded eBPF plugin limit (%d) reached", maxLoadedEBPFPlugins)
	}
	reservation := &ebpfPluginLoadReservation{}
	loadingPlugins[id] = reservation
	return reservation, nil
}

func releaseEBPFPluginLoad(id string, reservation *ebpfPluginLoadReservation) {
	loadedPluginsMu.Lock()
	if loadingPlugins[id] == reservation {
		delete(loadingPlugins, id)
	}
	loadedPluginsMu.Unlock()
}

func installLoadedEBPFPlugin(id string, reservation *ebpfPluginLoadReservation, entry *loadedEBPFPlugin) (*loadedEBPFPlugin, error) {
	loadedPluginsMu.Lock()
	defer loadedPluginsMu.Unlock()
	if loadingPlugins[id] != reservation || reservation.canceled {
		if loadingPlugins[id] == reservation {
			delete(loadingPlugins, id)
		}
		return nil, fmt.Errorf("plugin %q load was canceled", id)
	}
	old := loadedPlugins[id]
	loadedPlugins[id] = entry
	delete(loadingPlugins, id)
	return old, nil
}

// UnloadEBPFPlugin detaches and frees resources for a plugin.
func UnloadEBPFPlugin(id string) {
	loadedPluginsMu.Lock()
	if reservation := loadingPlugins[id]; reservation != nil {
		reservation.canceled = true
	}
	entry := loadedPlugins[id]
	delete(loadedPlugins, id)
	loadedPluginsMu.Unlock()
	closeLoadedPlugin(entry)
}

func closeLoadedPlugin(entry *loadedEBPFPlugin) {
	if entry == nil {
		return
	}
	for _, l := range entry.links {
		_ = l.Close()
	}
	if entry.collection != nil {
		entry.collection.Close()
	}
}

func splitTracepointTarget(target string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(target), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("tracepoint target must be category/name, got %q", target)
	}
	if !userBPFTracepointComponentPattern.MatchString(parts[0]) ||
		!userBPFTracepointComponentPattern.MatchString(parts[1]) ||
		strings.Contains(parts[0], "..") || strings.Contains(parts[1], "..") {
		return "", "", fmt.Errorf("invalid tracepoint target %q", target)
	}
	return parts[0], parts[1], nil
}

func loadedEBPFPluginActive(entry *loadedEBPFPlugin) bool {
	return entry != nil && (entry.collection != nil || len(entry.links) != 0)
}

func ebpfPluginRuntimeState(id string) (bool, string) {
	loadedPluginsMu.Lock()
	defer loadedPluginsMu.Unlock()
	entry, ok := loadedPlugins[id]
	if !ok {
		return false, ""
	}
	return loadedEBPFPluginActive(entry), entry.loadError
}

// ReapplyEBPFPluginsOnBoot brings up all enabled eBPF plugins after startup.
func ReapplyEBPFPluginsOnBoot() {
	for _, m := range pluginRegistry.List() {
		if m.Kind != PluginKindEBPF || !m.Enabled {
			continue
		}
		copyM := m
		if err := LoadEBPFPlugin(&copyM); err != nil {
			loadedPluginsMu.Lock()
			loadedPlugins[m.ID] = &loadedEBPFPlugin{loadError: err.Error()}
			loadedPluginsMu.Unlock()
		}
	}
}

// BPFTemplates returns a fixed set of starter snippets for the online builder.
type BPFTemplate struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	AttachKind   PluginAttachKind `json:"attachKind"`
	AttachTarget string           `json:"attachTarget"`
	ProgramName  string           `json:"programName"`
	Source       string           `json:"source"`
}

func bpfTemplates() []BPFTemplate {
	return []BPFTemplate{
		{
			ID:           "trace-execve",
			Name:         "Trace execve",
			Description:  "Logs every process exec via the sys_enter_execve tracepoint.",
			AttachKind:   PluginAttachTracepoint,
			AttachTarget: "syscalls/sys_enter_execve",
			ProgramName:  "trace_execve",
			Source: `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

SEC("tracepoint/syscalls/sys_enter_execve")
int trace_execve(void *ctx) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    bpf_printk("execve by %s\n", comm);
    return 0;
}
`,
		},
		{
			ID:           "count-openat",
			Name:         "Count openat",
			Description:  "Increments a per-CPU array entry every time openat is invoked.",
			AttachKind:   PluginAttachTracepoint,
			AttachTarget: "syscalls/sys_enter_openat",
			ProgramName:  "count_openat",
			Source: `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>

char LICENSE[] SEC("license") = "GPL";

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __type(key, u32);
    __type(value, u64);
    __uint(max_entries, 1);
} openat_count SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_openat")
int count_openat(void *ctx) {
    u32 key = 0;
    u64 *val = bpf_map_lookup_elem(&openat_count, &key);
    if (val) __sync_fetch_and_add(val, 1);
    return 0;
}
`,
		},
		{
			ID:           "kprobe-unlink",
			Name:         "Kprobe do_unlinkat",
			Description:  "Prints the comm of every process that calls do_unlinkat.",
			AttachKind:   PluginAttachKprobe,
			AttachTarget: "do_unlinkat",
			ProgramName:  "trace_unlink",
			Source: `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

SEC("kprobe/do_unlinkat")
int trace_unlink(struct pt_regs *ctx) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    bpf_printk("unlink by %s\n", comm);
    return 0;
}
`,
		},
	}
}
