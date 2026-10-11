package componentwire

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"agent-ebpf-filter/internal/componentipc"
)

func validEvent() Event {
	return Event{
		SchemaVersion: EnvelopeVersion,
		EventID:       "evt-test-1",
		Source:        "kernel",
		TimestampNS:   123456789,
		Type:          "exec",
		PID:           42,
	}
}

func TestEventProjectionRoundTrip(t *testing.T) {
	source := validEvent()
	data, err := EncodeEvent(source)
	if err != nil {
		t.Fatal(err)
	}
	source.EventID = "later-mutated"
	received, err := DecodeEvent(data)
	if err != nil {
		t.Fatal(err)
	}
	if received.EventID != "evt-test-1" || received.SchemaVersion != EnvelopeVersion ||
		received.Source != "kernel" || received.PID != 42 {
		t.Fatalf("decoded event: %+v", received)
	}
	if bytes.Contains(data, []byte("argv")) || bytes.Contains(data, []byte("prompt")) {
		t.Fatal("projection unexpectedly added sensitive raw fields")
	}
}

func TestEventProjectionFailsClosed(t *testing.T) {
	for _, mutate := range []func(*Event){
		func(e *Event) { e.SchemaVersion = "component.event.v2" },
		func(e *Event) { e.EventID = "" },
		func(e *Event) { e.EventID = strings.Repeat("x", 129) },
		func(e *Event) { e.Source = "  " },
		func(e *Event) { e.Type = "" },
		func(e *Event) { e.TimestampNS = 0 },
	} {
		bad := validEvent()
		mutate(&bad)
		if _, err := EncodeEvent(bad); !errors.Is(err, ErrInvalidEnvelope) {
			t.Fatalf("invalid event accepted: %+v: %v", bad, err)
		}
	}
	for _, raw := range [][]byte{
		nil,
		[]byte("{broken-json"),
		[]byte("{}"),
		[]byte("{\"schema_version\":\"component.event.v1\",\"event_id\":\"x\",\"source\":\"kernel\",\"timestamp_ns\":1,\"event_type\":\"exec\",\"argv\":\"SECRET\"}"),
		[]byte("{\"schema_version\":\"component.event.v1\",\"event_id\":\"x\",\"source\":\"kernel\",\"timestamp_ns\":1,\"event_type\":\"exec\"} {}"),
	} {
		if _, err := DecodeEvent(raw); !errors.Is(err, ErrInvalidEnvelope) {
			t.Fatalf("invalid JSON accepted: %q: %v", raw, err)
		}
	}
	if _, err := DecodeEvent(make([]byte, componentipc.MaxSequencedPayloadBytes+1)); !errors.Is(err, componentipc.ErrFrameTooLarge) {
		t.Fatalf("oversized projection accepted: %v", err)
	}
}
