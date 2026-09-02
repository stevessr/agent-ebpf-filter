package tls

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// ---- moved from backend/zz_merged_backend_test.go section probemanagertls_test.go ----

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
		{symbol: "SSL_write_ex", program: "uprobe_ssl_write_ex"},
		{symbol: "SSL_read", program: "uprobe_ssl_read"},
		{symbol: "SSL_read_ex", program: "uprobe_ssl_read_ex"},
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
		{symbol: "SSL_read_ex", program: "uretprobe_ssl_read_ex"},
		{symbol: "SSL_write_ex", program: "uretprobe_ssl_write_ex"},
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

	for _, symbol := range []string{"SSL_write", "gnutls_record_send", "PR_Write", "crypto/tls.(*Conn).Write"} {
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

func TestTLSProbeManagerDiscoveryLoopIsIdempotentAndStopsOnClose(t *testing.T) {
	manager := &TLSProbeManager{}
	var discoveryCalls atomic.Int32
	var duplicateCalls atomic.Int32
	manager.startGoDiscoveryLoop(2*time.Millisecond, func() {
		discoveryCalls.Add(1)
	})
	manager.startGoDiscoveryLoop(time.Millisecond, func() {
		duplicateCalls.Add(1)
	})

	deadline := time.Now().Add(time.Second)
	for discoveryCalls.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("discovery calls = %d, want at least 2", discoveryCalls.Load())
		}
		time.Sleep(time.Millisecond)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	stoppedAt := discoveryCalls.Load()
	time.Sleep(10 * time.Millisecond)
	if discoveryCalls.Load() != stoppedAt {
		t.Fatalf("discovery continued after Close: %d -> %d", stoppedAt, discoveryCalls.Load())
	}
	if duplicateCalls.Load() != 0 {
		t.Fatalf("duplicate discovery loop ran %d times", duplicateCalls.Load())
	}
	manager.startGoDiscoveryLoop(time.Millisecond, func() { duplicateCalls.Add(1) })
	time.Sleep(5 * time.Millisecond)
	if duplicateCalls.Load() != 0 {
		t.Fatalf("closed manager restarted discovery %d times", duplicateCalls.Load())
	}
}

func TestTLSCaptureControllerCloseResetsDiscoveryLifecycle(t *testing.T) {
	controller := &TLSCaptureController{
		manager:            &TLSProbeManager{},
		readStarted:        true,
		goDiscoveryStarted: true,
	}
	if err := controller.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	status := controller.Status()
	if status["enabled"] != false || status["readStarted"] != false || status["goDiscoveryStarted"] != false {
		t.Fatalf("controller status after Close = %#v", status)
	}
}

func TestTLSCaptureControllerCloseWaitsForReadLoop(t *testing.T) {
	done := make(chan struct{})
	controller := &TLSCaptureController{
		manager:     &TLSProbeManager{},
		readStarted: true,
		readDone:    done,
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(done)
		close(released)
	}()

	if err := controller.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-released:
	default:
		t.Fatal("Close() returned before the read loop exited")
	}
}

func TestTLSProbeManagerReadLoopStatsSnapshotIsAtomic(t *testing.T) {
	manager := &TLSProbeManager{}
	manager.readLoopStats.totalFrags.Add(3)
	manager.readLoopStats.droppedFrags.Add(1)
	manager.readLoopStats.completedFrags.Add(2)
	manager.readLoopStats.httpEvents.Add(1)
	manager.readLoopStats.rawEvents.Add(1)
	manager.readLoopStats.lastFragmentNS.Store(42)

	stats := manager.ReadLoopStatsSnapshot()
	if stats.TotalFrags != 3 || stats.DroppedFrags != 1 || stats.CompletedFrags != 2 || stats.HTTPEvents != 1 || stats.RawEvents != 1 || stats.LastFragmentNS != 42 {
		t.Fatalf("unexpected read-loop stats: %+v", stats)
	}
}

func TestResolveShebangInterpreterUsesEnvArgument(t *testing.T) {
	interpreter := resolveShebangInterpreter("/usr/bin/env sh -c echo")
	if interpreter == "" {
		t.Fatal("expected env sh to resolve to an executable")
	}
	switch base := filepath.Base(interpreter); base {
	case "sh", "bash", "dash":
		// All are valid resolutions for /usr/bin/env sh depending on distro.
	default:
		t.Fatalf("interpreter = %q, want a POSIX shell executable", interpreter)
	}
}

func TestResolveShebangInterpreterFallsBackToAbsoluteTarget(t *testing.T) {
	interpreter := resolveShebangInterpreter("/does/not/exist -S node")
	if interpreter != "/does/not/exist" {
		t.Fatalf("interpreter = %q, want absolute shebang target", interpreter)
	}
}
