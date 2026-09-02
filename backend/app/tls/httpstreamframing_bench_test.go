package tls

import "testing"

func BenchmarkValidateTLSHTTPHeaderFraming(b *testing.B) {
	payload := []byte("POST /v1/responses HTTP/1.1\r\nHost: api.example.com\r\nAuthorization: Bearer secret\r\nContent-Type: application/json\r\nContent-Length: 32\r\n\r\n{\"model\":\"example\",\"input\":[]}")
	b.ReportAllocs()
	for b.Loop() {
		complete, reason := validateTLSHTTPHeaderFraming(payload)
		if !complete || reason != "" {
			b.Fatalf("complete=%v reason=%q", complete, reason)
		}
	}
}
