package app

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/pb"
)

// ── Envelope event wrappers (migrated to app/events/) ──────────────────────

func normalizeCapturedEventRecord(record CapturedEventRecord) CapturedEventRecord {
	return events.NormalizeCapturedEventRecord(record)
}

func eventEnvelopeToJSONValue(envelope *pb.EventEnvelope) map[string]any {
	return events.EnvelopeToJSONValue(envelope)
}

func determineEventEnvelopeSource(event *pb.Event) string {
	return events.DetermineEnvelopeSource(event)
}

var eventEnvelopeJSONMarshaller = events.EnvelopeJSONMarshaller

func cloneProtoEvent(event *pb.Event) *pb.Event {
	return events.CloneProtoEvent(event)
}

func buildCapturedEventJSONRecords(records []CapturedEventRecord) []map[string]any {
	return events.BuildCapturedEventJSONRecords(records)
}