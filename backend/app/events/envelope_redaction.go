package events

import pb "agent-ebpf-filter/pb"

// EnvelopeRedactionState extracts the redaction state string from an event
// envelope regardless of payload kind.

func EnvelopeRedactionState(envelope *pb.EventEnvelope) string {
	if envelope == nil {
		return ""
	}
	switch payload := envelope.GetPayload().(type) {
	case *pb.EventEnvelope_TlsEvent:
		return payload.TlsEvent.GetRedactionState()
	case *pb.EventEnvelope_HttpEvent:
		return payload.HttpEvent.GetRedactionState()
	case *pb.EventEnvelope_SseEvent:
		return payload.SseEvent.GetRedactionState()
	case *pb.EventEnvelope_StdioEvent:
		return payload.StdioEvent.GetRedactionState()
	case *pb.EventEnvelope_AgentsightAlertEvent:
		return payload.AgentsightAlertEvent.GetRedactionState()
	}
	return ""
}
