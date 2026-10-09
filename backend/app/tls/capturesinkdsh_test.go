package tls

import (
	"testing"
	"time"

	"agent-ebpf-filter/app/dshinspector"
)

func TestDshInspectorSinkSanitizesAndLabelsUserspaceCapture(t *testing.T) {
	store := NewTLSCaptureStore(8)
	sink := NewDshInspectorSink(store, nil)
	sink.Handle(dshinspector.Event{
		Type: "http_request", RequestID: "req-1", Timestamp: time.Now().UTC(),
		Method: "POST", URL: "https://api.example.test/v1/messages?api_key=secret",
		Headers: map[string]string{"Authorization": "Bearer top-secret", "Content-Type": "application/json"},
		Body:    `{"token":"top-secret","message":"hello"}`, BodySize: 40, ContentType: "application/json",
	})
	events := store.Recent(1)
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
	event := events[0]
	if event.CaptureSource != "dsh_inspector" || event.CaptureRequestID != "req-1" || event.Lib != "dsh-inspector" {
		t.Fatalf("unexpected source labels: %#v", event)
	}
	if event.Headers["authorization"] != tlsRedactedValue {
		t.Fatalf("authorization = %q", event.Headers["authorization"])
	}
	if event.Body == "" || event.Body == `{"token":"top-secret","message":"hello"}` {
		t.Fatalf("body was not sanitized: %q", event.Body)
	}
	if event.URL == "https://api.example.test/v1/messages?api_key=secret" {
		t.Fatalf("URL was not sanitized: %q", event.URL)
	}
}
