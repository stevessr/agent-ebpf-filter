package tls

import (
	"strings"
	"testing"
	"time"
)

// BenchmarkTLSFragmentDecodeAndAssemble measures the per-sample cost of the
// perf read loop's hot path for a typical single-fragment TLS record.
func BenchmarkTLSFragmentDecodeAndAssemble(b *testing.B) {
	payload := strings.Repeat("GET /v1/messages HTTP/1.1\r\nHost: api.anthropic.com\r\n", 40)[:1400]
	fixture := newTestTLSFragmentAt(0, 1, len(payload), payload, uint64(time.Now().UnixNano()))
	raw := encodeTLSFragmentSampleForTest(&testing.T{}, fixture, true)
	assembler := NewFragmentAssembler(10 * time.Second)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		fragment, err := decodeTLSFragmentSample(raw)
		if err != nil {
			b.Fatal(err)
		}
		completed, ok := assembler.Add(fragment)
		if !ok || len(completed.Payload) != len(payload) {
			b.Fatalf("assembly failed: ok=%v", ok)
		}
	}
}
