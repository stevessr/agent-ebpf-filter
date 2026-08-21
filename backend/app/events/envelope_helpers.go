package events

import (
	"strconv"
	"strings"
	"time"

	pb "agent-ebpf-filter/pb"
)

// Envelope helpers shared by the app bridge and the research package.

func EnvelopeEventTypeName(envelope *pb.EventEnvelope, event *pb.Event) string {
	if envelope != nil {
		return envelope.GetEventType().String()
	}
	if event != nil {
		return event.GetEventType().String()
	}
	return ""
}

func ParseRecentEventTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed.UTC()
	}
	if millis, err := strconv.ParseInt(raw, 10, 64); err == nil && millis > 0 {
		if millis > 1_000_000_000_000_000 {
			return time.Unix(0, millis).UTC()
		}
		return time.UnixMilli(millis).UTC()
	}
	return time.Time{}
}
