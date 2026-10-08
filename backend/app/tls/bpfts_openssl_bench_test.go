package tls

import "testing"

func BenchmarkBpfTSOpenSSLDecodeToCompleted(b *testing.B) {
	raw := encodeCompactBpfTSOpenSSLFixture(&testing.T{}, 1400, bpfTSOpenSSLDirectionSend, tlsFuncSSLWrite)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		event, err := decodeBpfTSOpenSSLEvent(raw)
		if err != nil {
			b.Fatal(err)
		}
		if completed := bpfTSOpenSSLToCompleted(event); len(completed.Payload) != 1400 {
			b.Fatalf("payload len = %d", len(completed.Payload))
		}
	}
}
