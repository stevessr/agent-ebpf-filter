package tls

import (
	"os"
	"testing"
)

func TestParseProcStartTimeHandlesComplexComm(t *testing.T) {
	stat := []byte("123 (agent worker ) name) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 424242")
	startTime, ok := parseProcStartTime(stat)
	if !ok || startTime != 424242 {
		t.Fatalf("start time = (%d, %v), want (424242, true)", startTime, ok)
	}
}

func TestReadProcStartTimeForCurrentProcess(t *testing.T) {
	startTime, ok := readProcStartTime(os.Getpid())
	if !ok || startTime == 0 {
		t.Fatalf("current process start time = (%d, %v)", startTime, ok)
	}
}

func TestPruneAutoDiscoveryStateResetsReusedPID(t *testing.T) {
	pid := os.Getpid()
	currentStart, ok := readProcStartTime(pid)
	if !ok {
		t.Fatal("cannot read current process start time")
	}

	manager := &TLSProbeManager{
		attachedExec: map[int]string{pid: "rustls"},
		attachedGo: map[string]bool{
			goAttachKey("/tmp/old-agent", pid): true,
		},
		attachedStatic: map[string]bool{
			staticSSLAttachKey("/tmp/old-agent", pid):                          true,
			"pid\x00" + itoaPID(pid) + "\x00openssl\x00/usr/lib/libssl.so.3": true,
		},
	}
	defer dropDiscoveryRuntimeState(manager)

	identity := pidIdentityStateFor(manager)
	identity.mu.Lock()
	identity.startTimes[pid] = currentStart + 1
	identity.mu.Unlock()
	manager.markTLSCaptureObserved(pid, 123456789)

	manager.pruneAutoDiscoveryState()

	if _, exists := manager.attachedExec[pid]; exists {
		t.Fatal("reused PID remained in attachedExec")
	}
	for key := range manager.attachedGo {
		if attachedPID, ok := pidFromGoAttachKey(key); ok && attachedPID == pid {
			t.Fatalf("reused PID remained in attachedGo: %q", key)
		}
	}
	for key := range manager.attachedStatic {
		if attachedPID, ok := pidFromStaticAttachKey(key); ok && attachedPID == pid {
			t.Fatalf("reused PID remained in attachedStatic: %q", key)
		}
	}
	if _, observed := manager.tlsCaptureObservation(pid); observed {
		t.Fatal("capture observation survived PID reuse")
	}
	if status := manager.AutoDiscoveryStatus(); status.PIDReuseResets != 1 {
		t.Fatalf("PID reuse resets = %d, want 1", status.PIDReuseResets)
	}
}

func itoaPID(pid int) string {
	// Keep the test independent of attach-key implementation details while
	// avoiding fmt in a hot package test file.
	if pid == 0 {
		return "0"
	}
	buf := [32]byte{}
	i := len(buf)
	for pid > 0 {
		i--
		buf[i] = byte('0' + pid%10)
		pid /= 10
	}
	return string(buf[i:])
}
