package tls

import (
	"debug/elf"
	"fmt"
)

const (
	strippedSSLStrategyBoringSSLExact       = "boringssl-exact"
	strippedSSLStrategyOpenSSLCommonPrefix = "openssl-common-prefix"
)

type strippedSSLDiscovery struct {
	ReadOffset     uint64
	WriteOffset    uint64
	Strategy       string
	CandidateCount int
}

// discoverKnownStrippedSSL is the single fail-closed stripped OpenSSL/BoringSSL
// discovery primitive shared by the production TLS manager and the bpf-ts
// userspace resolver. The byte signatures below are x86_64 machine code; callers
// must not silently apply them to another ELF architecture.
func discoverKnownStrippedSSL(machine elf.Machine, data []byte) (strippedSSLDiscovery, error) {
	if machine != elf.EM_X86_64 {
		return strippedSSLDiscovery{}, fmt.Errorf(
			"safe stripped SSL offsets are unavailable for ELF machine %s",
			machine,
		)
	}

	// Exact BoringSSL machine-code signatures are stronger evidence than a
	// surviving symbol-name string. Accept them even when aggressive stripping
	// removed SSL_read/SSL_write strings from the binary.
	readOff := findBS(data, bsSSLRead.pattern)
	if readOff >= 0 {
		writeCenter := readOff + 0xCA0
		writeOff := findBSNear(data, bsSSLWrite.pattern, writeCenter, 0x10000)
		if writeOff >= 0 {
			return strippedSSLDiscovery{
				ReadOffset:  uint64(readOff),
				WriteOffset: uint64(writeOff),
				Strategy:    strippedSSLStrategyBoringSSLExact,
			}, nil
		}
	}

	// The OpenSSL common prologue is compiler-generated and therefore weak
	// evidence. Keep the existing fail-closed requirements: both SSL function
	// names must survive somewhere in the binary, there must be exactly two
	// candidates, and their distance must be plausible.
	if !binaryContainsSSLReadWriteStrings(data) {
		return strippedSSLDiscovery{}, fmt.Errorf(
			"no exact BoringSSL match and binary does not contain both SSL_read and SSL_write names",
		)
	}

	osslMask := buildMask(osslSSLCommonPrefix.pattern)
	matches := make([]int64, 0, 2)
	for i := int64(0); i <= int64(len(data))-int64(len(osslSSLCommonPrefix.pattern)); i++ {
		if matchMasked(data[i:], osslSSLCommonPrefix.pattern, osslMask) {
			matches = append(matches, i)
		}
	}
	writeOff, readOff, _, ok := selectUnambiguousOpenSSLPair(matches, true)
	if !ok {
		return strippedSSLDiscovery{}, fmt.Errorf(
			"no unambiguous stripped SSL_read/SSL_write offsets (%d common-prefix candidates)",
			len(matches),
		)
	}
	return strippedSSLDiscovery{
		ReadOffset:     uint64(readOff),
		WriteOffset:    uint64(writeOff),
		Strategy:       strippedSSLStrategyOpenSSLCommonPrefix,
		CandidateCount: len(matches),
	}, nil
}
