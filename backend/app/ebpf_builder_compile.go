package app

import (
	"agent-ebpf-filter/app/platform"
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

	"golang.org/x/sys/unix"
)

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
	dir, err := platform.SecureOpenOrCreateDir(pluginDir)
	if err != nil {
		return "", nil, fmt.Errorf("open plugin directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Chmod(0o700); err != nil {
		return "", nil, err
	}
	if err := platform.ChownArtifactFile(dir); err != nil {
		return "", nil, err
	}
	sourceFile, sourceTemp, err := platform.CreateTempSibling(dir, "source")
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
	sourceInput, err := platform.OpenBeneath(dir, sourceTemp, os.O_RDONLY, 0)
	if err != nil {
		return "", nil, err
	}
	defer sourceInput.Close()
	objectFile, objectTemp, err := platform.CreateTempSibling(dir, "object")
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
	if err := platform.ValidateRegularSingleLink(objectFile); err != nil {
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
	if err := platform.RejectNonRegularOrMultiLink(dir, "source.c"); err != nil {
		return "", out, fmt.Errorf("validate published source destination: %w", err)
	}
	if err := platform.RejectNonRegularOrMultiLink(dir, "program.o"); err != nil {
		return "", out, fmt.Errorf("validate published object destination: %w", err)
	}
	// Publish the executable object first. Until RecordCompile advances the
	// manifest checksum, a crash or partial publication therefore fails closed
	// instead of loading an old object under newly displayed source.
	if err := platform.ReplaceFileInDir(dir, objectTemp, "program.o"); err != nil {
		return "", out, err
	}
	objectPublished = true
	if err := platform.ReplaceFileInDir(dir, sourceTemp, "source.c"); err != nil {
		return "", out, err
	}
	sourcePublished = true
	return objectPath, out, nil
}
