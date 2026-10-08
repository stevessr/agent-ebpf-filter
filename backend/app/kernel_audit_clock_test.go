package app

import (
	"testing"
	"time"
)

func TestAlignKernelKtime(t *testing.T) {
	observed := time.Date(2026, 9, 13, 15, 0, 0, 0, time.UTC)
	got := alignKernelKtime(9_500_000_000, observed, 10_000_000_000)
	if got.Clock != "monotonic" {
		t.Fatalf("clock = %q, want monotonic", got.Clock)
	}
	if got.DelayNS != uint64(500*time.Millisecond) {
		t.Fatalf("delay = %d, want %d", got.DelayNS, 500*time.Millisecond)
	}
	wantCapture := observed.Add(-500 * time.Millisecond)
	if !got.CapturedAt.Equal(wantCapture) {
		t.Fatalf("captured = %v, want %v", got.CapturedAt, wantCapture)
	}
}

func TestAlignKernelKtimeRejectsFutureMonotonicValue(t *testing.T) {
	observed := time.Date(2026, 9, 13, 15, 0, 0, 0, time.UTC)
	got := alignKernelKtime(11_000, observed, 10_000)
	if got.Clock != "unknown" || got.DelayNS != 0 || !got.CapturedAt.Equal(observed) {
		t.Fatalf("future monotonic timestamp should fall back to ingest time: %+v", got)
	}
}
