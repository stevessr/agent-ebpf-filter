package app

import (
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
	"agent-ebpf-filter/redaction"
)

func strictRedactionEngineForTest() *redaction.RedactionEngine {
	return redaction.NewRedactionEngine(redaction.RedactionPolicy{
		Level:              redaction.RedactionLevelStrict,
		DefaultPlaceholder: "[REDACTED]",
	})
}

// A captured record's envelope shares the record event instead of holding a
// second deep copy; redaction must still reach both views exactly once.
func TestCapturedRecordEnvelopeSharesEventAndRedactsOnce(t *testing.T) {
	t.Parallel()
	secret := "sk-abcdefghijklmnopqrstuvwxyz0123456789"
	record := normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.Now().UTC(),
		Event:      &pb.Event{Type: "openat", Comm: "node", Path: "/tmp/x", ExtraInfo: "token=" + secret},
	})
	if record.Envelope == nil || record.Envelope.GetLegacyEvent() != record.Event {
		t.Fatal("envelope LegacyEvent does not share the record event")
	}

	engine := strictRedactionEngineForTest()
	record = redactCapturedEventRecord(record, engine)
	if got := record.Event.GetExtraInfo(); strings.Contains(got, secret) {
		t.Fatalf("record event still holds the secret: %q", got)
	}
	if got := record.Envelope.GetLegacyEvent().GetExtraInfo(); strings.Contains(got, secret) {
		t.Fatalf("envelope legacy event still holds the secret: %q", got)
	}
	if record.Event.GetRedactionLevel() != string(redaction.RedactionLevelStrict) {
		t.Fatalf("redaction level = %q", record.Event.GetRedactionLevel())
	}
}

func TestRedactEnvelopeEventExceptSkipsOnlyTheSharedEvent(t *testing.T) {
	t.Parallel()
	secret := "sk-abcdefghijklmnopqrstuvwxyz0123456789"
	engine := strictRedactionEngineForTest()

	shared := &pb.Event{Type: "openat", ExtraInfo: "token=" + secret}
	envelope := &pb.EventEnvelope{LegacyEvent: shared, Comm: "node"}
	redactEnvelopeEventExcept(envelope, engine, shared)
	if !strings.Contains(shared.GetExtraInfo(), secret) {
		t.Fatal("shared event was redacted through the envelope although it was marked as already redacted")
	}

	independent := &pb.Event{Type: "openat", ExtraInfo: "token=" + secret}
	envelope = &pb.EventEnvelope{LegacyEvent: independent}
	redactEnvelopeEventExcept(envelope, engine, shared)
	if strings.Contains(independent.GetExtraInfo(), secret) {
		t.Fatal("independent legacy event was not redacted")
	}
}
