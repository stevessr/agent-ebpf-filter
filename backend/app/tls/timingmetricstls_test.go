package tls

import (
	"testing"
	"time"
)

func TestTLSCaptureTimingStatsTrackDelayAndClockKinds(t *testing.T) {
	before := TLSCaptureTimingStatsSnapshot()

	recordTLSCaptureTimingObservation(bpfKtimeObservation{
		Captured: time.Unix(1, 0).UTC(),
		Ingested: time.Unix(1, 125).UTC(),
		DelayNS:  125,
		Clock:    "monotonic",
	})
	recordTLSCaptureTimingObservation(bpfKtimeObservation{
		Captured: time.Unix(2, 0).UTC(),
		Ingested: time.Unix(2, 250).UTC(),
		DelayNS:  250,
		Clock:    "unix_replay",
	})
	recordTLSCaptureTimingObservation(bpfKtimeObservation{
		Captured: time.Unix(3, 0).UTC(),
		Ingested: time.Unix(3, 50).UTC(),
		DelayNS:  50,
		Clock:    "unknown",
	})

	after := TLSCaptureTimingStatsSnapshot()
	if after.Samples != before.Samples+3 {
		t.Fatalf("Samples = %d, want %d", after.Samples, before.Samples+3)
	}
	if after.LastDelayNS != 50 {
		t.Fatalf("LastDelayNS = %d, want 50", after.LastDelayNS)
	}
	if after.MaxDelayNS < 250 {
		t.Fatalf("MaxDelayNS = %d, want at least 250", after.MaxDelayNS)
	}
	if after.MonotonicSamples != before.MonotonicSamples+1 {
		t.Fatalf("MonotonicSamples = %d, want %d", after.MonotonicSamples, before.MonotonicSamples+1)
	}
	if after.ReplaySamples != before.ReplaySamples+1 {
		t.Fatalf("ReplaySamples = %d, want %d", after.ReplaySamples, before.ReplaySamples+1)
	}
	if after.UnknownClock != before.UnknownClock+1 {
		t.Fatalf("UnknownClock = %d, want %d", after.UnknownClock, before.UnknownClock+1)
	}
}
