package tls

import "testing"

func TestAnnotateTLSAPIFingerprintHTTP1(t *testing.T) {
	event := &TLSPlaintextEvent{
		Type: "http_request", Direction: "send", Method: "POST",
		Host: "api.anthropic.com", URL: "/v1/messages?api_key=secret",
	}
	annotateTLSAPIFingerprint(event)
	if event.CaptureSource != "tls_plaintext" || event.AppProtocol != "http1" {
		t.Fatalf("capture metadata = %+v", event)
	}
	if event.RequestPath != "/v1/messages" || event.APIProfile != "anthropic.messages" || event.Vendor != "anthropic" {
		t.Fatalf("fingerprint = %+v", event)
	}
	if event.APIConfidence < 90 {
		t.Fatalf("confidence = %d", event.APIConfidence)
	}
}

func TestAnnotateTLSAPIFingerprintHTTP2(t *testing.T) {
	event := &TLSPlaintextEvent{
		Type: "http2_headers", Direction: "send", Method: "POST",
		Host: "generativelanguage.googleapis.com", URL: "/v1beta/models/gemini:streamGenerateContent?key=secret",
	}
	annotateTLSAPIFingerprint(event)
	if event.AppProtocol != "http2" || event.RequestPath != "/v1beta/models/gemini:streamGenerateContent" || event.APIProfile != "google-gemini.generate" {
		t.Fatalf("fingerprint = %+v", event)
	}
}
