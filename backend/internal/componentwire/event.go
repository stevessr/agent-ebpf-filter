// Package componentwire defines strict typed domain payloads on top of the
// independent componentipc transport. Only already-normalized, redacted
// EventEnvelope messages may cross the Collector -> Engine boundary.
package componentwire

import (
	"errors"
	"fmt"
	"strings"

	"agent-ebpf-filter/internal/componentipc"
	"agent-ebpf-filter/pb"
	"google.golang.org/protobuf/proto"
)

const EnvelopeVersion = "envelope.v1"

var ErrInvalidEnvelope = errors.New("invalid component event envelope")

// ValidateEventEnvelope checks the minimum stable fields required by the
// split-engine observer. It does NOT perform redaction: callers must sanitize
// data in the privileged Collector before invoking EncodeEventEnvelope.
func ValidateEventEnvelope(event *pb.EventEnvelope) error {
	if event == nil ||
		event.GetSchemaVersion() != EnvelopeVersion ||
		strings.TrimSpace(event.GetEventId()) == "" ||
		strings.TrimSpace(event.GetSource()) == "" ||
		event.GetTimestampNs() == 0 {
		return ErrInvalidEnvelope
	}
	return nil
}

// EncodeEventEnvelope validates and size-checks a canonical protobuf envelope.
// Unlike the transport's opaque payload, this strictly binds one domain schema.
func EncodeEventEnvelope(event *pb.EventEnvelope) ([]byte, error) {
	if err := ValidateEventEnvelope(event); err != nil {
		return nil, err
	}
	size := proto.Size(event)
	if size <= 0 || size > componentipc.MaxSequencedPayloadBytes {
		return nil, fmt.Errorf("%w: %d", componentipc.ErrFrameTooLarge, size)
	}
	data, err := proto.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	return data, nil
}

func DecodeEventEnvelope(data []byte) (*pb.EventEnvelope, error) {
	if len(data) == 0 {
		return nil, ErrInvalidEnvelope
	}
	if len(data) > componentipc.MaxSequencedPayloadBytes {
		return nil, componentipc.ErrFrameTooLarge
	}
	var event pb.EventEnvelope
	if err := proto.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	if err := ValidateEventEnvelope(&event); err != nil {
		return nil, err
	}
	return &event, nil
}
