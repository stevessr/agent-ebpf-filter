package componentwire

import (
	"errors"
	"testing"

	"agent-ebpf-filter/internal/componentipc"
	"agent-ebpf-filter/pb"
)

func validEnvelope() *pb.EventEnvelope {
	return &pb.EventEnvelope{
		SchemaVersion: EnvelopeVersion,
		EventId: "evt-test-1",
		Source: "kernel",
		TimestampNs: 123456789,
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	source := validEnvelope()
	data, err := EncodeEventEnvelope(source)
	if err != nil {
		t.Fatal(err)
	}
	source.EventId = "later-mutated"
	received, err := DecodeEventEnvelope(data)
	if err != nil {
		t.Fatal(err)
	}
	if received.GetEventId() != "evt-test-1" ||
		received.GetSchemaVersion() != EnvelopeVersion ||
		received.GetSource() != "kernel" {
		t.Fatalf("decoded envelope: %+v", received)
	}
}

func TestEnvelopeFailClosedValidation(t *testing.T) {
	if _, err := EncodeEventEnvelope(nil); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("nil envelope accepted: %v", err)
	}
	for _, mutate := range []func(*pb.EventEnvelope){
		func(e *pb.EventEnvelope) { e.SchemaVersion = "envelope.v2" },
		func(e *pb.EventEnvelope) { e.EventId = "" },
		func(e *pb.EventEnvelope) { e.Source = "  " },
		func(e *pb.EventEnvelope) { e.TimestampNs = 0 },
	} {
		bad := validEnvelope()
		mutate(bad)
		if _, err := EncodeEventEnvelope(bad); !errors.Is(err, ErrInvalidEnvelope) {
			t.Fatalf("invalid envelope accepted: %+v: %v", bad, err)
		}
	}
	if _, err := DecodeEventEnvelope([]byte{0xff}); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("malformed protobuf accepted: %v", err)
	}
	if _, err := DecodeEventEnvelope(make([]byte, componentipc.MaxSequencedPayloadBytes+1)); !errors.Is(err, componentipc.ErrFrameTooLarge) {
		t.Fatalf("oversized protobuf accepted: %v", err)
	}
}
