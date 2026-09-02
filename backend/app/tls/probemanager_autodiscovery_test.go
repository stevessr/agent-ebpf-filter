package tls

import (
	"debug/elf"
	"os"
	"testing"
)

func TestStaticAndGoAttachReservationsAreIndependent(t *testing.T) {
	manager := &TLSProbeManager{
		attachedGo:     make(map[string]bool),
		attachedStatic: make(map[string]bool),
	}
	const path = "/tmp/agent"
	const pid = 42

	if !manager.shouldAttachGoBinary(path, pid) {
		t.Fatal("first Go reservation should be allowed")
	}
	if !manager.shouldAttachStaticSSL(path, pid) {
		t.Fatal("static TLS reservation must not be blocked by the Go reservation")
	}
	if manager.shouldAttachGoBinary(path, pid) {
		t.Fatal("duplicate Go reservation should be rejected")
	}
	if manager.shouldAttachStaticSSL(path, pid) {
		t.Fatal("duplicate static TLS reservation should be rejected")
	}
}

func TestPIDFromStaticAttachKeySupportsExecutableAndSharedLibraryKeys(t *testing.T) {
	tests := []struct {
		key  string
		want int
		ok   bool
	}{
		{key: "exec\x0042\x00/usr/bin/node", want: 42, ok: true},
		{key: "pid\x0043\x00openssl\x00/usr/lib/libssl.so.3", want: 43, ok: true},
		{key: "openssl\x00/usr/lib/libssl.so.3", want: 0, ok: false},
		{key: "pid\x00oops\x00openssl\x00/usr/lib/libssl.so.3", want: 0, ok: false},
	}
	for _, tt := range tests {
		got, ok := pidFromStaticAttachKey(tt.key)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("pidFromStaticAttachKey(%q) = (%d, %v), want (%d, %v)", tt.key, got, ok, tt.want, tt.ok)
		}
	}
}

func TestIsAgentTLSProcessRecognizesModernAgentRuntimes(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		cmdline string
		want    bool
	}{
		{name: "claude native", base: "claude", want: true},
		{name: "codex native", base: "codex", want: true},
		{name: "opencode native", base: "opencode", want: true},
		{name: "node claude", base: "node", cmdline: "node /x/@anthropic-ai/claude-code/cli.js", want: true},
		{name: "bun opencode", base: "bun", cmdline: "bun /opt/opencode/bin", want: true},
		{name: "generic node", base: "node", cmdline: "node server.js", want: false},
		{name: "generic python", base: "python3", cmdline: "python3 app.py", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAgentTLSProcess(tt.base, tt.cmdline); got != tt.want {
				t.Fatalf("isAgentTLSProcess(%q, %q) = %v, want %v", tt.base, tt.cmdline, got, tt.want)
			}
		})
	}
}

func TestFindLoadedLibForTargetPrefersLibSSL(t *testing.T) {
	loaded := []string{
		"/usr/lib/libcrypto.so.3",
		"/usr/lib/libssl.so.3",
	}
	path, ok := findLoadedLibForTarget(loaded, staticTLSLibraries[0])
	if !ok {
		t.Fatal("expected OpenSSL target to resolve")
	}
	if path != "/usr/lib/libssl.so.3" {
		t.Fatalf("resolved %q, want libssl", path)
	}
}

func TestVirtualAddressToFileOffset(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	exe, err := elf.Open(path)
	if err != nil {
		t.Fatalf("elf.Open: %v", err)
	}
	defer exe.Close()

	for _, program := range exe.Progs {
		if program == nil || program.Type != elf.PT_LOAD || program.Filesz < 2 {
			continue
		}
		address := program.Vaddr + 1
		got, ok := virtualAddressToFileOffset(exe, address)
		if !ok {
			t.Fatalf("failed to map load address %#x", address)
		}
		want := program.Off + 1
		if got != want {
			t.Fatalf("file offset = %#x, want %#x", got, want)
		}
		return
	}
	t.Fatal("test executable has no suitable PT_LOAD segment")
}
