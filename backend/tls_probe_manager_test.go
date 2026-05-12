package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindFirstExistingPath(t *testing.T) {
	tmpDir := t.TempDir()
	existing := filepath.Join(tmpDir, "libssl.so")
	if err := os.WriteFile(existing, []byte(""), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	got, ok := findFirstExistingPath("/does/not/exist", existing, filepath.Join(tmpDir, "missing"))
	if !ok {
		t.Fatalf("expected to find existing path")
	}
	if got != existing {
		t.Fatalf("got path %q, want %q", got, existing)
	}

	if _, ok := findFirstExistingPath("/still/missing", filepath.Join(tmpDir, "also-missing")); ok {
		t.Fatalf("expected missing paths to return false")
	}
}

func TestTLSProgramForSymbol(t *testing.T) {
	tests := []struct {
		symbol  string
		program string
	}{
		{symbol: "SSL_write", program: "uprobe_ssl_write"},
		{symbol: "SSL_write_ex", program: "uprobe_ssl_write"},
		{symbol: "SSL_read", program: "uprobe_ssl_read"},
		{symbol: "SSL_read_ex", program: "uprobe_ssl_read"},
		{symbol: "gnutls_record_send", program: "uprobe_gnutls_record_send"},
		{symbol: "gnutls_record_recv", program: "uprobe_gnutls_record_recv"},
		{symbol: "PR_Write", program: "uprobe_pr_write"},
		{symbol: "PR_Read", program: "uprobe_pr_read"},
		{symbol: "crypto/tls.(*Conn).Write", program: "uprobe_crypto_tls_conn_write"},
		{symbol: "crypto/tls.(*Conn).Read", program: "uprobe_crypto_tls_conn_read"},
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			got, ok := tlsProgramForSymbol(tt.symbol)
			if !ok {
				t.Fatalf("expected program for symbol %q", tt.symbol)
			}
			if got != tt.program {
				t.Fatalf("got program %q, want %q", got, tt.program)
			}
		})
	}
}

func TestTLSReturnProgramForSymbol(t *testing.T) {
	tests := []struct {
		symbol  string
		program string
	}{
		{symbol: "SSL_read", program: "uretprobe_ssl_read"},
		{symbol: "SSL_read_ex", program: "uretprobe_ssl_read"},
		{symbol: "gnutls_record_recv", program: "uretprobe_gnutls_record_recv"},
		{symbol: "PR_Read", program: "uretprobe_pr_read"},
		{symbol: "crypto/tls.(*Conn).Read", program: "uretprobe_crypto_tls_conn_read"},
	}
	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			got, ok := tlsReturnProgramForSymbol(tt.symbol)
			if !ok {
				t.Fatalf("expected return program for symbol %q", tt.symbol)
			}
			if got != tt.program {
				t.Fatalf("got return program %q, want %q", got, tt.program)
			}
		})
	}

	for _, symbol := range []string{"SSL_write", "SSL_write_ex", "gnutls_record_send", "PR_Write", "crypto/tls.(*Conn).Write"} {
		t.Run(symbol+" no return", func(t *testing.T) {
			if got, ok := tlsReturnProgramForSymbol(symbol); ok {
				t.Fatalf("return program for %q = %q, want none", symbol, got)
			}
		})
	}
}

func TestParseProcPID(t *testing.T) {
	pid, ok := parseProcPID("/proc/1234/exe")
	if !ok || pid != 1234 {
		t.Fatalf("pid = %d ok = %v", pid, ok)
	}

	if pid, ok := parseProcPID("/proc/self/exe"); ok || pid != 0 {
		t.Fatalf("self parsed as pid = %d ok = %v", pid, ok)
	}
}

func TestShouldAttachGoBinaryOnlyOncePerPIDPath(t *testing.T) {
	manager := &TLSProbeManager{attachedGo: make(map[string]bool)}
	if !manager.shouldAttachGoBinary("/tmp/app", 42) {
		t.Fatalf("first attach should be allowed")
	}
	if manager.shouldAttachGoBinary("/tmp/app", 42) {
		t.Fatalf("duplicate attach should be skipped")
	}
	if !manager.shouldAttachGoBinary("/tmp/app", 43) {
		t.Fatalf("different pid should be allowed")
	}
}

func TestForgetGoBinaryAttachAllowsRetryAfterFailure(t *testing.T) {
	manager := &TLSProbeManager{attachedGo: make(map[string]bool)}
	if !manager.shouldAttachGoBinary("/tmp/app", 42) {
		t.Fatalf("first attach should be allowed")
	}
	manager.forgetGoBinaryAttach("/tmp/app", 42)
	if !manager.shouldAttachGoBinary("/tmp/app", 42) {
		t.Fatalf("attach should be retried after failure cleanup")
	}
}
