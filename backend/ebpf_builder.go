package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

// loadedEBPFPlugin tracks an attached user plugin so we can detach on disable/delete.
type loadedEBPFPlugin struct {
	collection *ebpf.Collection
	links      []link.Link
	loadError  string
}

var (
	loadedPluginsMu sync.Mutex
	loadedPlugins   = make(map[string]*loadedEBPFPlugin)
)

// suspiciousIncludePattern blocks headers that would let user programs poke
// kernel internals beyond what the BPF helper surface already exposes. The
// allow-list is deliberately tight; the online builder is a low-trust surface.
var suspiciousIncludePattern = regexp.MustCompile(`(?m)^\s*#include\s*[<"](?:fcntl\.h|sys/.*|unistd\.h|stdio\.h|stdlib\.h|fs/.*|asm/.*|net/.*)[>"]`)

// validateUserBPFSource performs lightweight checks before we hand the source
// off to clang. We are not a sandbox — but rejecting obvious abuse keeps the
// resulting object file deterministic and small.
func validateUserBPFSource(source string) error {
	if strings.TrimSpace(source) == "" {
		return errors.New("source is empty")
	}
	if len(source) > 256*1024 {
		return errors.New("source exceeds 256 KiB")
	}
	if suspiciousIncludePattern.MatchString(source) {
		return errors.New("source includes a disallowed header (only bpf_*/linux/bpf.h-style includes are accepted)")
	}
	if !strings.Contains(source, "SEC(") {
		return errors.New("source must declare at least one SEC(\"...\") program")
	}
	return nil
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
	if err := validateUserBPFSource(source); err != nil {
		return "", nil, err
	}
	clang, err := clangBinary()
	if err != nil {
		return "", nil, err
	}
	if err := writePluginSource(pluginID, source); err != nil {
		return "", nil, err
	}
	src := pluginSourcePath(pluginID)
	obj := pluginObjectPath(pluginID)

	args := []string{
		"-O2", "-g", "-Wall",
		"-target", "bpf",
		"-D__TARGET_ARCH_x86",
		"-c", src,
		"-o", obj,
	}
	if dir := vmlinuxIncludeDir(); dir != "" {
		args = append([]string{"-I", dir}, args...)
	}
	cmd := exec.Command(clang, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", out, fmt.Errorf("clang failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if _, statErr := os.Stat(obj); statErr != nil {
		return "", out, fmt.Errorf("clang produced no object: %w", statErr)
	}
	if os.Getuid() == 0 {
		if uid, gid, ok := originalInvokerIDs(); ok {
			_ = os.Chown(obj, int(uid), int(gid))
		}
	}
	return obj, out, nil
}

// LoadEBPFPlugin reads the object file from disk and attaches the requested program.
func LoadEBPFPlugin(m *PluginManifest) error {
	if m == nil || m.Kind != PluginKindEBPF {
		return errors.New("not an eBPF plugin")
	}
	if m.AttachKind == "" || m.AttachKind == PluginAttachNone {
		return errors.New("attach kind is required")
	}
	if strings.TrimSpace(m.ProgramName) == "" {
		return errors.New("programName is required")
	}
	objPath := pluginObjectPath(m.ID)
	if _, err := os.Stat(objPath); err != nil {
		return fmt.Errorf("plugin object missing: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpec(objPath)
	if err != nil {
		return fmt.Errorf("load collection spec: %w", err)
	}
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("instantiate collection: %w", err)
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

	loadedPluginsMu.Lock()
	if old := loadedPlugins[m.ID]; old != nil {
		closeLoadedPlugin(old)
	}
	loadedPlugins[m.ID] = &loadedEBPFPlugin{
		collection: coll,
		links:      []link.Link{attached},
	}
	loadedPluginsMu.Unlock()
	return nil
}

// UnloadEBPFPlugin detaches and frees resources for a plugin.
func UnloadEBPFPlugin(id string) {
	loadedPluginsMu.Lock()
	defer loadedPluginsMu.Unlock()
	if entry := loadedPlugins[id]; entry != nil {
		closeLoadedPlugin(entry)
		delete(loadedPlugins, id)
	}
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
	return parts[0], parts[1], nil
}

func ebpfPluginRuntimeState(id string) (bool, string) {
	loadedPluginsMu.Lock()
	defer loadedPluginsMu.Unlock()
	entry, ok := loadedPlugins[id]
	if !ok {
		return false, ""
	}
	return true, entry.loadError
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
