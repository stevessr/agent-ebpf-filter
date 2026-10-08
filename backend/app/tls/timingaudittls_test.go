package tls

import (
	"testing"
	"time"
)

func TestObserveBPFKtimePreservesReplayUnixTimestamp(t *testing.T) {
	captured := time.Now().UTC().Add(-25 * time.Millisecond).Truncate(time.Nanosecond)
	observation := observeBPFKtime(uint64(captured.UnixNano()))

	if observation.Clock != "unix_replay" {
		t.Fatalf("clock = %q, want unix_replay", observation.Clock)
	}
	if !observation.Captured.Equal(captured) {
		t.Fatalf("captured = %s, want %s", observation.Captured, captured)
	}
	if observation.Ingested.Before(observation.Captured) {
		t.Fatalf("ingested %s is before captured %s", observation.Ingested, observation.Captured)
	}
	if observation.DelayNS == 0 {
		t.Fatalf("expected non-zero replay capture delay")
	}
}

func TestObserveBPFKtimeZeroIsExplicitlyUnknown(t *testing.T) {
	observation := observeBPFKtime(0)
	if observation.Clock != "unknown" {
		t.Fatalf("clock = %q, want unknown", observation.Clock)
	}
	if observation.Captured.IsZero() || observation.Ingested.IsZero() {
		t.Fatalf("zero timestamp fallback must remain observable")
	}
	if !observation.Captured.Equal(observation.Ingested) {
		t.Fatalf("zero timestamp fallback should use ingest time")
	}
	if observation.DelayNS != 0 {
		t.Fatalf("delay = %d, want 0 for unknown probe time", observation.DelayNS)
	}
}

func TestApplyTLSCaptureTimingAnnotatesAuditTuple(t *testing.T) {
	captured := time.Now().UTC().Add(-10 * time.Millisecond).Truncate(time.Nanosecond)
	rawNS := uint64(captured.UnixNano())
	event := TLSPlaintextEvent{Type: "tls_plaintext"}

	applyTLSCaptureTiming(&event, rawNS)

	if event.ProbeTimestampNS != rawNS {
		t.Fatalf("probe timestamp = %d, want %d", event.ProbeTimestampNS, rawNS)
	}
	if event.ProbeClock != "unix_replay" {
		t.Fatalf("probe clock = %q, want unix_replay", event.ProbeClock)
	}
	if !event.Timestamp.Equal(captured) {
		t.Fatalf("event timestamp = %s, want %s", event.Timestamp, captured)
	}
	if event.IngestTimestamp.IsZero() {
		t.Fatalf("expected ingest timestamp")
	}
	if event.CaptureDelayNS == 0 {
		t.Fatalf("expected positive capture delay")
	}
}

func TestCompletedProcessorAddsTimingToRawEvent(t *testing.T) {
	store := NewTLSCaptureStore(8)
	processor := newTLSCompletedEventProcessor(store, NewTLSCaptureRuleStore(), NewTLSCaptureBroadcaster())
	captured := time.Now().UTC().Add(-5 * time.Millisecond).Truncate(time.Nanosecond)
	payload := []byte{0x01, 0x02, 0x03, 0x04}
	completed := CompletedTLSFragment{
		TimestampNS: uint64(captured.UnixNano()),
		PID:         123,
		TGID:        123,
		TotalLen:    uint32(len(payload)),
		OriginalLen: uint32(len(payload)),
		FragCount:   1,
		LibType:     tlsLibOpenSSL,
		Direction:   tlsDirectionSend,
		Function:    tlsFuncSSLWrite,
		Comm:        "curl",
		Payload:     payload,
	}

	result := processor.Process(completed)
	if result.RawEvents != 1 {
		t.Fatalf("raw events = %d, want 1", result.RawEvents)
	}
	events := store.Recent(1)
	if len(events) != 1 {
		t.Fatalf("stored events = %d, want 1", len(events))
	}
	got := events[0]
	if got.ProbeTimestampNS != completed.TimestampNS || got.ProbeClock != "unix_replay" {
		t.Fatalf("missing timing provenance: %+v", got)
	}
	if got.IngestTimestamp.IsZero() || got.CaptureDelayNS == 0 {
		t.Fatalf("missing ingest timing: %+v", got)
	}
}
