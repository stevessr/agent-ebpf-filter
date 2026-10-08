package tls

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func BenchmarkHexDumpRawPayload(b *testing.B) {
	payload := bytes.Repeat([]byte{0x16, 0x03, 0x01, 0xde, 0xad, 0xbe, 0xef, 0x00}, 512)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for b.Loop() {
		if len(hexDump(payload)) == 0 {
			b.Fatal("empty dump")
		}
	}
}

func legacyHexDump(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var b strings.Builder
	for i, v := range payload {
		if i > 0 {
			b.WriteByte(' ')
		}
		_, _ = fmt.Fprintf(&b, "%02x", v)
	}
	return b.String()
}

func TestHexDumpMatchesLegacyFormat(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, n := range []int{0, 1, 2, 3, 17, 512, 4096} {
		payload := make([]byte, n)
		rng.Read(payload)
		if got, want := hexDump(payload), legacyHexDump(payload); got != want {
			t.Fatalf("hexDump(%d bytes) = %q, want %q", n, got, want)
		}
	}
	if got := hexDump([]byte{0x00, 0xff, 0x0a}); got != "00 ff 0a" {
		t.Fatalf("hexDump = %q", got)
	}
}
