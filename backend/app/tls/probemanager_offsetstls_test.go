package tls

import "testing"

func TestSelectUnambiguousOpenSSLPairAcceptsExactlyTwoPlausibleCandidates(t *testing.T) {
	writeOff, readOff, distance, ok := selectUnambiguousOpenSSLPair([]int64{0x1000, 0x1800}, true)
	if !ok {
		t.Fatal("expected unambiguous pair to be accepted")
	}
	if writeOff != 0x1000 || readOff != 0x1800 || distance != 0x800 {
		t.Fatalf("pair = (%#x, %#x, %#x), want (0x1000, 0x1800, 0x800)", writeOff, readOff, distance)
	}
}

func TestSelectUnambiguousOpenSSLPairRejectsMultipleCandidates(t *testing.T) {
	if _, _, _, ok := selectUnambiguousOpenSSLPair([]int64{0x1000, 0x1800, 0x2200}, true); ok {
		t.Fatal("ambiguous common-prologue candidates were accepted")
	}
}

func TestSelectUnambiguousOpenSSLPairRejectsMissingSSLNames(t *testing.T) {
	if _, _, _, ok := selectUnambiguousOpenSSLPair([]int64{0x1000, 0x1800}, false); ok {
		t.Fatal("pair without SSL_read/SSL_write evidence was accepted")
	}
}

func TestSelectUnambiguousOpenSSLPairRejectsImplausibleDistance(t *testing.T) {
	for name, matches := range map[string][]int64{
		"too_close": {0x1000, 0x1080},
		"too_far":   {0x1000, 0x12000},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, ok := selectUnambiguousOpenSSLPair(matches, true); ok {
				t.Fatalf("implausible pair %v was accepted", matches)
			}
		})
	}
}
