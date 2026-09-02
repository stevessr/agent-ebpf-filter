package tls

import (
	"errors"
	"testing"
	"time"
)

func TestTLSRetryDelayIsExponentialAndCapped(t *testing.T) {
	if got := tlsRetryDelay(1); got != 15*time.Second {
		t.Fatalf("first retry delay = %s, want 15s", got)
	}
	if got := tlsRetryDelay(2); got != 30*time.Second {
		t.Fatalf("second retry delay = %s, want 30s", got)
	}
	if got := tlsRetryDelay(3); got != time.Minute {
		t.Fatalf("third retry delay = %s, want 1m", got)
	}
	if got := tlsRetryDelay(20); got != tlsAutoAttachRetryMax {
		t.Fatalf("capped retry delay = %s, want %s", got, tlsAutoAttachRetryMax)
	}
}

func TestAutoAttachBackoffSkipsUntilRetryWindow(t *testing.T) {
	manager := &TLSProbeManager{}
	defer dropDiscoveryRuntimeState(manager)

	now := time.Unix(1000, 0)
	const (
		kind = "agent"
		pid  = 42
		path = "/tmp/agent"
	)
	if !manager.autoAttachAllowed(kind, pid, path, now) {
		t.Fatal("first attach should be allowed")
	}
	manager.recordAutoAttachAttempt()
	manager.recordAutoAttachFailure(kind, pid, path, errors.New("unsupported TLS layout"), now)

	if manager.autoAttachAllowed(kind, pid, path, now.Add(5*time.Second)) {
		t.Fatal("attach inside retry window should be skipped")
	}
	if !manager.autoAttachAllowed(kind, pid, path, now.Add(tlsAutoAttachRetryBase)) {
		t.Fatal("attach should be allowed when retry window expires")
	}

	status := manager.AutoDiscoveryStatus()
	if status.Attempts != 1 || status.Failures != 1 || status.BackoffSkips != 1 {
		t.Fatalf("unexpected status after failure: %+v", status)
	}
	if status.LastError != "unsupported TLS layout" {
		t.Fatalf("last error = %q", status.LastError)
	}

	manager.recordAutoAttachSuccess(kind, pid, path)
	if !manager.autoAttachAllowed(kind, pid, path, now.Add(tlsAutoAttachRetryBase)) {
		t.Fatal("successful attach should clear retry state")
	}
	if status := manager.AutoDiscoveryStatus(); status.Successes != 1 {
		t.Fatalf("successes = %d, want 1", status.Successes)
	}
}

func TestTLSCaptureObservationMarksActualProbeActivity(t *testing.T) {
	manager := &TLSProbeManager{}
	defer dropDiscoveryRuntimeState(manager)

	const pid = 1234
	if _, observed := manager.tlsCaptureObservation(pid); observed {
		t.Fatal("PID should start unobserved")
	}
	manager.markTLSCaptureObserved(pid, 1234567890)
	lastNS, observed := manager.tlsCaptureObservation(pid)
	if !observed || lastNS != 1234567890 {
		t.Fatalf("observation = (%d, %v), want (1234567890, true)", lastNS, observed)
	}
	if status := manager.AutoDiscoveryStatus(); status.ObservedPIDs != 1 {
		t.Fatalf("observed PIDs = %d, want 1", status.ObservedPIDs)
	}
}
