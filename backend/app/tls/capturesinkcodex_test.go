package tls

import (
	"testing"

	codexhandlers "agent-ebpf-filter/codex/capture/handlers"
)

func TestCodexCaptureSinkPreservesResponsesContextMetadata(t *testing.T) {
	store := NewTLSCaptureStore(4)
	sink := NewCodexCaptureSink(store, nil)

	sink.HandleCaptureEvent(codexhandlers.Event{
		Type:               "websocket_request",
		PID:                42,
		TGID:               42,
		Comm:               "codex",
		Direction:          "send",
		Host:               "api.openai.com",
		URL:                "wss://api.openai.com/v1/responses",
		ProtocolEvent:      "response.create",
		StreamID:           "lane-1",
		ResponseID:         "resp_2",
		PreviousResponseID: "resp_1",
		PromptDigest:       "sha256:prompt",
		PromptLen:          11,
		ContextDigest:      "sha256:context",
		ContextLen:         128,
		ContextItems:       3,
		Vendor:             "codex",
	})

	recent := store.Recent(1)
	if len(recent) != 1 {
		t.Fatalf("recent events = %d, want 1", len(recent))
	}
	event := recent[0]
	if event.ProtocolEvent != "response.create" ||
		event.StreamID != "lane-1" ||
		event.ResponseID != "resp_2" ||
		event.PreviousResponseID != "resp_1" {
		t.Fatalf("Responses identifiers lost in capture sink: %#v", event)
	}
	if event.ContextDigest != "sha256:context" ||
		event.ContextLen != 128 ||
		event.ContextItems != 3 {
		t.Fatalf("Responses context metadata lost in capture sink: %#v", event)
	}
}
