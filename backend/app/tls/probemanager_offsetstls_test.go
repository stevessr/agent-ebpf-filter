package tls

import (
	"debug/elf"
	"testing"
)

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

func TestDiscoverKnownStrippedSSLRejectsNonX86Machine(t *testing.T) {
	data, _, _ := exactBoringSSLFixture(false)
	if _, err := discoverKnownStrippedSSL(elf.EM_AARCH64, data); err == nil {
		t.Fatal("x86_64 stripped signatures were accepted for arm64")
	}
}

func TestDiscoverKnownStrippedSSLExactPatternNeedsNoNames(t *testing.T) {
	data, readOff, writeOff := exactBoringSSLFixture(false)
	discovery, err := discoverKnownStrippedSSL(elf.EM_X86_64, data)
	if err != nil {
		t.Fatalf("discoverKnownStrippedSSL() error = %v", err)
	}
	if discovery.Strategy != strippedSSLStrategyBoringSSLExact {
		t.Fatalf("strategy = %q, want %q", discovery.Strategy, strippedSSLStrategyBoringSSLExact)
	}
	if discovery.ReadOffset != uint64(readOff) || discovery.WriteOffset != uint64(writeOff) {
		t.Fatalf("offsets = (%#x, %#x), want (%#x, %#x)", discovery.ReadOffset, discovery.WriteOffset, readOff, writeOff)
	}
}

func TestDiscoverKnownStrippedSSLReportsFailClosedOpenSSLStrategy(t *testing.T) {
	data := make([]byte, 0x6000)
	writeOff := 0x1000
	readOff := 0x1800
	copy(data[writeOff:], osslSSLCommonPrefix.pattern)
	copy(data[readOff:], osslSSLCommonPrefix.pattern)
	copy(data[0x5000:], []byte("SSL_read\x00SSL_write\x00"))

	discovery, err := discoverKnownStrippedSSL(elf.EM_X86_64, data)
	if err != nil {
		t.Fatalf("discoverKnownStrippedSSL() error = %v", err)
	}
	if discovery.Strategy != strippedSSLStrategyOpenSSLCommonPrefix {
		t.Fatalf("strategy = %q, want %q", discovery.Strategy, strippedSSLStrategyOpenSSLCommonPrefix)
	}
	if discovery.CandidateCount != 2 {
		t.Fatalf("candidate count = %d, want 2", discovery.CandidateCount)
	}
	if discovery.WriteOffset != uint64(writeOff) || discovery.ReadOffset != uint64(readOff) {
		t.Fatalf("offsets = (%#x, %#x), want (%#x, %#x)", discovery.WriteOffset, discovery.ReadOffset, writeOff, readOff)
	}
}
