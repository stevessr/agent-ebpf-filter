// Package componentwire defines a small, privacy-minimized domain projection
// for the future Collector -> Engine boundary. It imports neither the
// privileged Collector runtime nor generated protobuf code.
package componentwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"agent-ebpf-filter/internal/componentipc"
)

const EnvelopeVersion = "component.event.v1"

var ErrInvalidEnvelope = errors.New("invalid component event projection")

// Event is an explicitly whitelisted, read-only telemetry projection.
// It intentionally contains no raw argv, file contents, prompts, secrets,
// network payloads, paths or other arbitrary kernel data. Conversion from
// backend pb.EventEnvelope belongs in the privileged, redacting Collector.
type Event struct {
	SchemaVersion string `json:"schema_version"`
	EventID       string `json:"event_id"`
	Source        string `json:"source"`
	TimestampNS   uint64 `json:"timestamp_ns"`
	Type          string `json:"event_type"`
	PID           uint32 `json:"pid,omitempty"`
	Decision      string `json:"decision,omitempty"`
	RiskScore     uint32 `json:"risk_score,omitempty"`
}

// ValidateEvent ensures the projection is both bounded and structurally
// meaningful; it does not assert that existing policy decisions are correct.
func ValidateEvent(event Event) error {
	if event.SchemaVersion != EnvelopeVersion || event.TimestampNS == 0 ||
		strings.TrimSpace(event.EventID) == "" || len(event.EventID) > 128 ||
		strings.TrimSpace(event.Source) == "" || len(event.Source) > 64 ||
		strings.TrimSpace(event.Type) == "" || len(event.Type) > 64 ||
		len(event.Decision) > 64 {
		return ErrInvalidEnvelope
	}
	return nil
}

// EncodeEvent projects one already-sanitized event into a bounded, strictly
// versioned schema. Callers may not add arbitrary or sensitive fields.
func EncodeEvent(event Event) ([]byte, error) {
	if err := ValidateEvent(event); err != nil {
		return nil, err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	if len(data) > componentipc.MaxSequencedPayloadBytes {
		return nil, componentipc.ErrFrameTooLarge
	}
	return data, nil
}

// DecodeEvent rejects unknown fields, trailing JSON, missing mandatory fields
// and unsupported versions. It avoids accidental acceptance of raw envelopes
// containing sensitive fields outside the limited intercomponent schema.
func DecodeEvent(data []byte) (Event, error) {
	if len(data) == 0 {
		return Event{}, ErrInvalidEnvelope
	}
	if len(data) > componentipc.MaxSequencedPayloadBytes {
		return Event{}, componentipc.ErrFrameTooLarge
	}
	reader := json.NewDecoder(bytes.NewReader(data))
	reader.DisallowUnknownFields()
	var event Event
	if err := reader.Decode(&event); err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	var next any
	if err := reader.Decode(&next); !errors.Is(err, io.EOF) {
		return Event{}, ErrInvalidEnvelope
	}
	if err := ValidateEvent(event); err != nil {
		return Event{}, err
	}
	return event, nil
}
