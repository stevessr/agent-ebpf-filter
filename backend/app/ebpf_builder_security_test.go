package app

import (
	"agent-ebpf-filter/app/handlers"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cilium/ebpf"
)

const validTestBPFSource = `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
char LICENSE[] SEC("license") = "GPL";
SEC("tracepoint/syscalls/sys_enter_execve") int test_prog(void *ctx) { return 0; }
`

func writeFakeClang(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clang")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+script+"\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(fake clang) error = %v", err)
	}
	return path
}

func fakeClangOutputScript(body string) string {
	return `out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then shift; out="$1"; fi
  shift
done
test -n "$out"
` + body
}

func TestValidateUserBPFSourceRejectsFilesystemDirectives(t *testing.T) {
	for _, directive := range []string{
		`#include "/etc/passwd"`,
		`#include <../secret.h>`,
		`#include SECRET_HEADER`,
		`#include_next <bpf/bpf_helpers.h>`,
		`#import "/tmp/secret"`,
		`#embed "/tmp/secret"`,
		`#/**/include "/etc/passwd"`,
		`#include/**/ "/etc/passwd"`,
		`#if __has_include("/etc/passwd")`,
		`asm(".incbin \"/tmp/secret\"");`,
		"#inc\\\nlude \"/etc/passwd\"",
		`%:include "/etc/passwd"`,
		`??=include "/etc/passwd"`,
		`asm(".inc" "bin \"/tmp/secret\"");`,
		"#define X SEC(\"x\")\x00",
	} {
		source := directive + "\n" + validTestBPFSource
		if err := validateUserBPFSource(source); err == nil {
			t.Fatalf("unsafe directive accepted: %s", directive)
		}
	}
	if err := validateUserBPFSource(validTestBPFSource); err != nil {
		t.Fatalf("allowed includes rejected: %v", err)
	}
}

func TestCompileUserBPFRejectsInvalidIDDirectly(t *testing.T) {
	if _, _, err := CompileUserBPF("../../escape", validTestBPFSource); err == nil {
		t.Fatal("CompileUserBPF accepted traversal id")
	}
}

func TestCompileUserBPFContextCancelsWhileQueued(t *testing.T) {
	clang := writeFakeClang(t, `exit 99`)
	t.Setenv("PATH", filepath.Dir(clang))
	oldSlots := userBPFCompileSlots
	userBPFCompileSlots = make(chan struct{}, 1)
	userBPFCompileSlots <- struct{}{}
	t.Cleanup(func() { userBPFCompileSlots = oldSlots })
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(20*time.Millisecond, cancel)
	defer timer.Stop()
	_, _, err := CompileUserBPFContext(ctx, "safe-plugin", validTestBPFSource)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("queued cancellation error = %v", err)
	}
}

func TestHandlerCompileRegistersDraftAndArtifactDigest(t *testing.T) {
	root := withPluginRoot(t)
	oldRegistry := pluginRegistry
	pluginRegistry = &pluginStore{entries: make(map[string]*PluginManifest)}
	t.Cleanup(func() { pluginRegistry = oldRegistry })
	clang := writeFakeClang(t, fakeClangOutputScript(`printf 'compiled-object' > "$out"`))
	t.Setenv("PATH", filepath.Dir(clang))
	objectPath, _, err := handlers.Deps.CompileUserBPF(context.Background(), "draft-plugin", validTestBPFSource)
	if err != nil {
		t.Fatalf("handler compile error = %v", err)
	}
	if objectPath != filepath.Join(root, "draft-plugin", "program.o") {
		t.Fatalf("object path = %q", objectPath)
	}
	manifest, ok := pluginRegistry.Get("draft-plugin")
	if !ok || manifest.Kind != PluginKindEBPF || manifest.AttachKind != PluginAttachNone {
		t.Fatalf("compile draft manifest = %+v, found=%v", manifest, ok)
	}
	if manifest.SourceSHA256 != sha256Hex([]byte(validTestBPFSource)) || manifest.ObjectSHA256 != sha256Hex([]byte("compiled-object")) {
		t.Fatalf("compile digests not recorded: %+v", manifest)
	}
}

func TestCompileUserBPFPublishesPrivateFilesAtomically(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugin-safe")
	clang := writeFakeClang(t, fakeClangOutputScript(`printf 'object-data' > "$out"; printf 'warning' >&2`))
	objectPath := filepath.Join(dir, "program.o")
	gotPath, diagnostics, err := compileUserBPFInDir(context.Background(), clang, "plugin-safe", validTestBPFSource, dir, objectPath)
	if err != nil {
		t.Fatalf("compileUserBPFInDir() error = %v diagnostics=%q", err, diagnostics)
	}
	if gotPath != objectPath || !strings.Contains(string(diagnostics), "warning") {
		t.Fatalf("result path=%q diagnostics=%q", gotPath, diagnostics)
	}
	for name, want := range map[string]string{"source.c": validTestBPFSource, "program.o": "object-data"} {
		payload, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(payload) != want {
			t.Fatalf("%s payload=%q error=%v", name, payload, err)
		}
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode=%v error=%v", name, info.Mode(), err)
		}
	}
}

func TestCompileUserBPFWithRealClangFileDescriptors(t *testing.T) {
	clang, err := clangBinary()
	if err != nil {
		t.Skip(err)
	}
	source := `#define SEC(name) __attribute__((section(name), used))
SEC("tracepoint/syscalls/sys_enter_execve") int test_prog(void *ctx) { return 0; }
char _license[] SEC("license") = "GPL";
`
	dir := filepath.Join(t.TempDir(), "plugin-safe")
	_, output, err := compileUserBPFInDir(context.Background(), clang, "plugin-safe", source, dir, filepath.Join(dir, "program.o"))
	if err != nil {
		if strings.Contains(string(output), "No available targets are compatible with triple") {
			t.Skipf("clang lacks BPF target: %s", output)
		}
		t.Fatalf("real clang compile failed: %v output=%s", err, output)
	}
	info, err := os.Stat(filepath.Join(dir, "program.o"))
	if err != nil || info.Size() == 0 {
		t.Fatalf("real clang object missing: info=%v err=%v", info, err)
	}
	spec, err := ebpf.LoadCollectionSpec(filepath.Join(dir, "program.o"))
	if err != nil {
		t.Fatalf("parse real clang object: %v", err)
	}
	manifest := testTracepointManifest()
	if _, err := validateUserBPFCollectionSpec(spec, manifest); err != nil {
		t.Fatalf("validate real clang object: %v", err)
	}
}

func TestCompileUserBPFFailurePreservesPublishedFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugin-safe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, payload := range map[string]string{"source.c": "old-source", "program.o": "old-object"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	clang := writeFakeClang(t, `printf 'compile failed' >&2; exit 1`)
	if _, _, err := compileUserBPFInDir(context.Background(), clang, "plugin-safe", validTestBPFSource, dir, filepath.Join(dir, "program.o")); err == nil {
		t.Fatal("failed compiler was accepted")
	}
	for name, want := range map[string]string{"source.c": "old-source", "program.o": "old-object"} {
		payload, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(payload) != want {
			t.Fatalf("%s changed after failed compile: %q, %v", name, payload, err)
		}
	}
}

func TestCompileUserBPFRejectsUnsafePublishedDestination(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugin-safe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "program.o")); err != nil {
		t.Fatal(err)
	}
	clang := writeFakeClang(t, fakeClangOutputScript(`printf 'object-data' > "$out"`))
	if _, _, err := compileUserBPFInDir(context.Background(), clang, "plugin-safe", validTestBPFSource, dir, filepath.Join(dir, "program.o")); err == nil {
		t.Fatal("compile accepted symlinked published object")
	}
	payload, err := os.ReadFile(outside)
	if err != nil || string(payload) != "unchanged" {
		t.Fatalf("outside target changed: %q, %v", payload, err)
	}
}

func TestCompileUserBPFBoundsTimeoutDiagnosticsAndObject(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		clang := writeFakeClang(t, `sleep 2`)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, _, err := compileUserBPFInDir(ctx, clang, "plugin-safe", validTestBPFSource, filepath.Join(t.TempDir(), "plugin"), "program.o")
		if err == nil || !strings.Contains(err.Error(), "timed out or canceled") {
			t.Fatalf("timeout error = %v", err)
		}
	})

	t.Run("diagnostics", func(t *testing.T) {
		clang := writeFakeClang(t, `head -c 2097152 /dev/zero | tr '\\000' x >&2; exit 1`)
		_, output, err := compileUserBPFInDir(context.Background(), clang, "plugin-safe", validTestBPFSource, filepath.Join(t.TempDir(), "plugin"), "program.o")
		if err == nil || len(output) > maxUserBPFDiagnosticsBytes+64 || !strings.Contains(string(output), "truncated") {
			t.Fatalf("diagnostics len=%d error=%v suffix=%q", len(output), err, output[len(output)-min(len(output), 64):])
		}
	})

	t.Run("object", func(t *testing.T) {
		clang := writeFakeClang(t, fakeClangOutputScript(`truncate -s 33554433 "$out"`))
		_, _, err := compileUserBPFInDir(context.Background(), clang, "plugin-safe", validTestBPFSource, filepath.Join(t.TempDir(), "plugin"), "program.o")
		if err == nil || !strings.Contains(err.Error(), "object size") {
			t.Fatalf("oversized object error = %v", err)
		}
	})
}
