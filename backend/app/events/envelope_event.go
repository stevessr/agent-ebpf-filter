package events

import (
	"agent-ebpf-filter/pb"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const eventEnvelopeSchemaVersion = "envelope.v1"

var EnvelopeJSONMarshaller = protojson.MarshalOptions{
	UseProtoNames:   true,
	EmitUnpopulated: false,
}

func NormalizeCapturedEventRecord(record CapturedEventRecord) CapturedEventRecord {
	if record.Event == nil && record.Envelope != nil && record.Envelope.GetLegacyEvent() != nil {
		record.Event = CloneProtoEvent(record.Envelope.GetLegacyEvent())
	}
	if record.Envelope == nil {
		record.Envelope = buildEventEnvelope(record)
	} else {
		record.Envelope = normalizeEventEnvelope(record.Envelope, record)
	}
	return record
}

func CloneProtoEvent(event *pb.Event) *pb.Event {
	if event == nil {
		return nil
	}
	cloned, ok := proto.Clone(event).(*pb.Event)
	if ok {
		return cloned
	}
	return event
}

func normalizeEventEnvelope(envelope *pb.EventEnvelope, record CapturedEventRecord) *pb.EventEnvelope {
	cloned, ok := proto.Clone(envelope).(*pb.EventEnvelope)
	if !ok {
		cloned = envelope
	}
	if cloned.GetLegacyEvent() == nil && record.Event != nil {
		cloned.LegacyEvent = CloneProtoEvent(record.Event)
	}
	if strings.TrimSpace(cloned.GetSchemaVersion()) == "" {
		cloned.SchemaVersion = eventEnvelopeSchemaVersion
	}
	legacy := firstNonNilEvent(record.Event, cloned.GetLegacyEvent())
	if cloned.GetTimestampNs() == 0 {
		if legacy != nil && legacy.GetCaptureTimestampNs() != 0 {
			cloned.TimestampNs = legacy.GetCaptureTimestampNs()
		} else {
			timestamp := record.ReceivedAt.UTC()
			if timestamp.IsZero() {
				timestamp = time.Now().UTC()
			}
			cloned.TimestampNs = uint64(timestamp.UnixNano())
		}
	}
	if legacy != nil {
		if cloned.GetKernelTimestampNs() == 0 {
			cloned.KernelTimestampNs = legacy.GetKernelTimestampNs()
		}
		if cloned.GetKernelSequence() == 0 {
			cloned.KernelSequence = legacy.GetKernelSequence()
		}
		if cloned.GetKernelCpu() == 0 {
			cloned.KernelCpu = legacy.GetKernelCpu()
		}
		if strings.TrimSpace(cloned.GetKernelClock()) == "" {
			cloned.KernelClock = legacy.GetKernelClock()
		}
		if cloned.GetIngestTimestampNs() == 0 {
			cloned.IngestTimestampNs = legacy.GetIngestTimestampNs()
		}
		if cloned.GetCaptureDelayNs() == 0 {
			cloned.CaptureDelayNs = legacy.GetCaptureDelayNs()
		}
		if cloned.GetCaptureTimestampNs() == 0 {
			cloned.CaptureTimestampNs = legacy.GetCaptureTimestampNs()
		}
		if cloned.GetAuditFlags() == 0 {
			cloned.AuditFlags = legacy.GetAuditFlags()
		}
		if cloned.GetKernelAuditGeneration() == 0 {
			cloned.KernelAuditGeneration = legacy.GetKernelAuditGeneration()
		}
		if cloned.GetKernelDroppedSinceLast() == 0 {
			cloned.KernelDroppedSinceLast = legacy.GetKernelDroppedSinceLast()
		}
		if cloned.GetKernelReserveFailuresTotal() == 0 {
			cloned.KernelReserveFailuresTotal = legacy.GetKernelReserveFailuresTotal()
		}
	}
	if strings.TrimSpace(cloned.GetSource()) == "" {
		cloned.Source = DetermineEnvelopeSource(record.Event)
	}
	if strings.TrimSpace(cloned.GetEventId()) == "" {
		cloned.EventId = buildEventEnvelopeID(record, legacy)
	}
	return cloned
}

func firstNonNilEvent(candidates ...*pb.Event) *pb.Event {
	for _, candidate := range candidates {
		if candidate != nil {
			return candidate
		}
	}
	return nil
}

// buildEventEnvelope derives the envelope for record.Event. The envelope's
// LegacyEvent shares record.Event rather than cloning it: a captured record
// already owns a private copy of the event, both views are only ever
// serialised together, and redaction (redactCapturedEventRecord) is applied
// to the shared object exactly once.
func buildEventEnvelope(record CapturedEventRecord) *pb.EventEnvelope {
	event := record.Event
	if event == nil {
		return nil
	}
	timestamp := record.ReceivedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	timestampNS := uint64(timestamp.UnixNano())
	if event.GetCaptureTimestampNs() != 0 {
		timestampNS = event.GetCaptureTimestampNs()
	}
	envelope := &pb.EventEnvelope{
		SchemaVersion:              eventEnvelopeSchemaVersion,
		TimestampNs:                timestampNS,
		Source:                     DetermineEnvelopeSource(event),
		AgentRunId:                 event.GetAgentRunId(),
		TaskId:                     event.GetTaskId(),
		ConversationId:             event.GetConversationId(),
		TurnId:                     event.GetTurnId(),
		ToolCallId:                 event.GetToolCallId(),
		ToolName:                   event.GetToolName(),
		TraceId:                    event.GetTraceId(),
		SpanId:                     event.GetSpanId(),
		Pid:                        event.GetPid(),
		Tgid:                       tgidOrPid(event),
		Ppid:                       event.GetPpid(),
		Uid:                        event.GetUid(),
		Gid:                        event.GetGid(),
		Comm:                       event.GetComm(),
		ArgvDigest:                 event.GetArgvDigest(),
		Cwd:                        event.GetCwd(),
		CgroupId:                   event.GetCgroupId(),
		ContainerId:                event.GetContainerId(),
		PolicyDecision:             event.GetDecision(),
		RiskScore:                  event.GetRiskScore(),
		EventType:                  event.GetEventType(),
		KernelTimestampNs:          event.GetKernelTimestampNs(),
		KernelSequence:             event.GetKernelSequence(),
		KernelCpu:                  event.GetKernelCpu(),
		KernelClock:                event.GetKernelClock(),
		IngestTimestampNs:          event.GetIngestTimestampNs(),
		CaptureDelayNs:             event.GetCaptureDelayNs(),
		CaptureTimestampNs:         event.GetCaptureTimestampNs(),
		AuditFlags:                 event.GetAuditFlags(),
		KernelAuditGeneration:      event.GetKernelAuditGeneration(),
		KernelDroppedSinceLast:     event.GetKernelDroppedSinceLast(),
		KernelReserveFailuresTotal: event.GetKernelReserveFailuresTotal(),
		LegacyEvent:                event,
	}
	envelope.EventId = buildEventEnvelopeID(record, event)

	setEnvelopePayload(envelope, event)
	return envelope
}

func buildEventEnvelopeID(record CapturedEventRecord, event *pb.Event) string {
	if event == nil {
		return ""
	}

	// Kernel provenance is stable across persistence/replay and across
	// userspace scheduling delays. Prefer it whenever an explicit generation
	// and CPU-local sequence are available.
	if event.GetKernelAuditGeneration() != 0 && event.GetKernelSequence() != 0 {
		parts := []string{
			"kernel-v2",
			fmt.Sprintf("%d", event.GetKernelAuditGeneration()),
			strconv.FormatUint(uint64(event.GetKernelCpu()), 10),
			fmt.Sprintf("%d", event.GetKernelSequence()),
			fmt.Sprintf("%d", event.GetKernelTimestampNs()),
			event.GetType(),
			strconv.FormatUint(uint64(event.GetPid()), 10),
			strconv.FormatUint(uint64(tgidOrPid(event)), 10),
		}
		sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
		return "evt_" + hex.EncodeToString(sum[:12])
	}

	timestamp := record.ReceivedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Unix(0, 0).UTC()
	}
	// The hashed material is the NUL-joined field list; it is assembled in a
	// stack scratch buffer so typical events hash without allocating.
	var scratch [512]byte
	buf := strconv.AppendInt(scratch[:0], timestamp.UnixNano(), 10)
	buf = append(buf, 0)
	buf = append(buf, DetermineEnvelopeSource(event)...)
	buf = append(buf, 0)
	buf = append(buf, event.GetType()...)
	buf = append(buf, 0)
	buf = strconv.AppendUint(buf, uint64(event.GetPid()), 10)
	buf = append(buf, 0)
	buf = strconv.AppendUint(buf, uint64(event.GetPpid()), 10)
	for _, part := range [...]string{
		event.GetComm(),
		event.GetPath(),
		event.GetNetEndpoint(),
		event.GetTraceId(),
		event.GetToolCallId(),
		event.GetDecision(),
		event.GetExtraInfo(),
	} {
		buf = append(buf, 0)
		buf = append(buf, part...)
	}
	sum := sha256.Sum256(buf)
	var id [4 + 24]byte
	copy(id[:], "evt_")
	hex.Encode(id[4:], sum[:12])
	return string(id[:])
}

func BuildCapturedEventJSONRecords(records []CapturedEventRecord) []map[string]any {
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		record = NormalizeCapturedEventRecord(record)
		items = append(items, map[string]any{
			"Event":     record.Event,
			"Timestamp": record.ReceivedAt.UnixMilli(),
			"Envelope":  EnvelopeToJSONValue(record.Envelope),
		})
	}
	return items
}

func EnvelopeToJSONValue(envelope *pb.EventEnvelope) map[string]any {
	if envelope == nil {
		return nil
	}
	payload, err := EnvelopeJSONMarshaller.Marshal(envelope)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	decoded := make(map[string]any)
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return decoded
}

func tgidOrPid(event *pb.Event) uint32 {
	if tgid := event.GetTgid(); tgid != 0 {
		return tgid
	}
	return event.GetPid()
}

// RecentEventFilters narrows a recent-events query. Zero fields are ignored.
type RecentEventFilters struct {
	Type           string
	EventType      string
	Source         string
	PID            uint32
	Comm           string
	TraceID        string
	SpanID         string
	RedactionState string
	Since          time.Time
	Until          time.Time
}

// IsZero reports whether the filter matches everything.
func (f RecentEventFilters) IsZero() bool { return f == RecentEventFilters{} }

// RecentEventFiltersFromRequest reads the filter fields from query parameters.
func RecentEventFiltersFromRequest(c *gin.Context) RecentEventFilters {
	filters := RecentEventFilters{
		Type:           strings.TrimSpace(c.Query("type")),
		EventType:      strings.TrimSpace(c.Query("event_type")),
		Source:         strings.TrimSpace(c.Query("source")),
		Comm:           strings.TrimSpace(c.Query("comm")),
		TraceID:        strings.TrimSpace(c.Query("trace_id")),
		SpanID:         strings.TrimSpace(c.Query("span_id")),
		RedactionState: strings.TrimSpace(c.Query("redaction_state")),
	}
	if filters.EventType == "" {
		filters.EventType = strings.TrimSpace(c.Query("eventType"))
	}
	if raw := strings.TrimSpace(c.Query("pid")); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 32); err == nil {
			filters.PID = uint32(parsed)
		}
	}
	filters.Since = ParseRecentEventTime(c.Query("since"))
	filters.Until = ParseRecentEventTime(c.Query("until"))
	return filters
}
