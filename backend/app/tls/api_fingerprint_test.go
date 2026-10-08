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

func TestAnnotateTLSAPIFingerprintGRPC(t *testing.T) {
	event := &TLSPlaintextEvent{
		Type: "http2_headers", Direction: "send", Method: "POST",
		Host: "grpc.example", URL: "/acme.agent.v1.AgentService/Run?token=secret",
		ContentType: "application/grpc+proto",
	}
	annotateTLSAPIFingerprint(event)
	if event.AppProtocol != "grpc" || event.RequestPath != "/acme.agent.v1.AgentService/Run" {
		t.Fatalf("grpc normalization = %+v", event)
	}
	if event.APIProduct != "acme.agent.v1.AgentService" || event.APIOperation != "Run" || event.APIConfidence != 70 {
		t.Fatalf("grpc fingerprint = %+v", event)
	}
}

func TestAnnotateTLSAPIFingerprintMiniMaxProviderNotHarness(t *testing.T) {
	event := &TLSPlaintextEvent{
		Type: "http2_headers", Direction: "send", Method: "POST",
		Host: "api.minimax.io", URL: "/anthropic/v1/messages?api_key=secret",
		Comm: "zcode",
	}
	annotateTLSAPIFingerprint(event)
	if event.RequestPath != "/anthropic/v1/messages" || event.APIProfile != "minimax.messages" ||
		event.Vendor != "minimax" || event.APIProduct != "messages" {
		t.Fatalf("MiniMax provider detection must be independent of calling harness: %+v", event)
	}
}
