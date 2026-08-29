package tls

import "testing"

func TestSaturatingSumUint64(t *testing.T) {
	if got := saturatingSumUint64([]uint64{1, 2, 3, 5}); got != 11 {
		t.Fatalf("sum = %d, want 11", got)
	}
	if got := saturatingSumUint64([]uint64{^uint64(0) - 1, 10}); got != ^uint64(0) {
		t.Fatalf("overflow sum = %d, want saturation", got)
	}
}

func TestBpfTSOpenSSLBackpressureStatusWithoutRuntime(t *testing.T) {
	bridge := NewBpfTSOpenSSLBridgeRuntime(nil, nil, nil)
	status := bridge.BackpressureStatus()
	if status.KernelDrops != 0 || status.AttemptedRecords != 0 || status.DropPercent != 0 || status.Error != "" {
		t.Fatalf("unexpected dormant backpressure status: %#v", status)
	}
}
