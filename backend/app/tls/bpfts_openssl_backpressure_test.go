package tls

import (
	"math"
	"testing"
)

func TestSaturatingSumUint64(t *testing.T) {
	if got := saturatingSumUint64([]uint64{1, 2, 3, 5}); got != 11 {
		t.Fatalf("sum = %d, want 11", got)
	}
	if got := saturatingSumUint64([]uint64{^uint64(0) - 1, 10}); got != ^uint64(0) {
		t.Fatalf("overflow sum = %d, want saturation", got)
	}
}

func TestBpfTSOpenSSLBackpressureSeparatesLossCauses(t *testing.T) {
	status := bpfTSOpenSSLBackpressureFromCounts(90, 10, 5)
	if status.KernelDrops != 10 || status.UserReadErrors != 5 {
		t.Fatalf("unexpected loss causes: %#v", status)
	}
	if status.AttemptedRecords != 100 {
		t.Fatalf("attemptedRecords = %d, want 100", status.AttemptedRecords)
	}
	if status.TotalLosses != 15 || status.CaptureAttempts != 105 {
		t.Fatalf("unexpected total loss accounting: %#v", status)
	}
	if math.Abs(status.DropPercent-10) > 1e-9 {
		t.Fatalf("dropPercent = %f, want 10", status.DropPercent)
	}
	wantTotalLossPercent := float64(15) * 100 / 105
	if math.Abs(status.TotalLossPercent-wantTotalLossPercent) > 1e-9 {
		t.Fatalf("totalLossPercent = %f, want %f", status.TotalLossPercent, wantTotalLossPercent)
	}
}

func TestBpfTSOpenSSLBackpressureSaturatesCounters(t *testing.T) {
	status := bpfTSOpenSSLBackpressureFromCounts(^uint64(0)-1, 10, 10)
	if status.AttemptedRecords != ^uint64(0) || status.TotalLosses != 20 || status.CaptureAttempts != ^uint64(0) {
		t.Fatalf("unexpected saturated accounting: %#v", status)
	}
}

func TestBpfTSOpenSSLBackpressureStatusWithoutRuntime(t *testing.T) {
	bridge := NewBpfTSOpenSSLBridgeRuntime(nil, nil, nil)
	status := bridge.BackpressureStatus()
	if status.KernelDrops != 0 || status.UserReadErrors != 0 || status.AttemptedRecords != 0 || status.TotalLosses != 0 || status.CaptureAttempts != 0 || status.DropPercent != 0 || status.TotalLossPercent != 0 || status.Error != "" {
		t.Fatalf("unexpected dormant backpressure status: %#v", status)
	}
}
