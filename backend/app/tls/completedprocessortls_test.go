package tls

import "testing"

func testProcessorCompletedTLSFragment(payload string) CompletedTLSFragment {
	return CompletedTLSFragment{
		TimestampNS:  1,
		ConnectionID: 99,
		PID:          123,
		TGID:         123,
		DataLen:      uint32(len(payload)),
		TotalLen:     uint32(len(payload)),
		OriginalLen:  uint32(len(payload)),
		FragCount:    1,
		LibType:      tlsLibOpenSSL,
		Direction:    tlsDirectionSend,
		Function:     tlsFuncSSLWrite,
		Comm:         "agent",
		Payload:      []byte(payload),
	}
}

func TestTLSCompletedEventProcessorStoresRawFallback(t *testing.T) {
	store := NewTLSCaptureStore(16)
	processor := newTLSCompletedEventProcessor(store, NewTLSCaptureRuleStore(), NewTLSCaptureBroadcaster())
	result := processor.Process(testProcessorCompletedTLSFragment("opaque-agent-protocol"))
	if result.RawEvents != 1 || result.HTTPEvents != 0 {
		t.Fatalf("result = %+v", result)
	}
	if store.Count() != 1 {
		t.Fatalf("store count = %d, want 1", store.Count())
	}
	event := store.Recent(1)[0]
	if event.Type != "tls_plaintext" || event.Function != "SSL_write" || event.Direction != "send" {
		t.Fatalf("unexpected raw event: %+v", event)
	}
}

func TestTLSCompletedEventProcessorSuppressesRecognizedPartialHTTP(t *testing.T) {
	store := NewTLSCaptureStore(16)
	processor := newTLSCompletedEventProcessor(store, NewTLSCaptureRuleStore(), NewTLSCaptureBroadcaster())
	result := processor.Process(testProcessorCompletedTLSFragment("GET /v1/messages HTTP/1.1\r\nHost: api.example.test\r\nContent-Length: 12\r\n\r\nhello"))
	if result.RawEvents != 0 || result.HTTPEvents != 0 {
		t.Fatalf("partial HTTP result = %+v", result)
	}
	if store.Count() != 0 {
		t.Fatalf("recognized partial HTTP leaked raw event; store count = %d", store.Count())
	}
}
