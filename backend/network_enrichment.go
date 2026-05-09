package main

import (
	"net"
	"strings"
)

// ── IP Address Classification (from rustnet bogon.rs) ─────────────────

type IPScope string

const (
	ScopePublic        IPScope = "Public"
	ScopeLoopback      IPScope = "Loopback"
	ScopePrivate       IPScope = "Private"
	ScopeLinkLocal     IPScope = "Link-Local"
	ScopeCGNAT         IPScope = "CGNAT"
	ScopeMulticast     IPScope = "Multicast"
	ScopeBroadcast     IPScope = "Broadcast"
	ScopeDocumentation IPScope = "Documentation"
	ScopeBenchmarking  IPScope = "Benchmarking"
	ScopeUnspecified   IPScope = "Unspecified"
	ScopeReserved      IPScope = "Reserved"
	ScopeUniqueLocal   IPScope = "Unique-Local"
	ScopeDiscard       IPScope = "Discard"
	ScopeIPv4Mapped    IPScope = "IPv4-Mapped"
	ScopeUnknown       IPScope = "Unknown"
)

func classifyIPScope(ip net.IP) IPScope {
	if ip == nil {
		return ScopeUnknown
	}
	if ip4 := ip.To4(); ip4 != nil {
		return classifyIPv4Scope(ip4)
	}
	return classifyIPv6Scope(ip.To16())
}

func classifyIPv4Scope(ip net.IP) IPScope {
	if ip == nil || len(ip) < 4 {
		return ScopeUnknown
	}
	b0, b1 := ip[0], ip[1]

	// 0.0.0.0/8 - "This host on this network"
	if b0 == 0 {
		if ip.Equal(net.IPv4zero) {
			return ScopeUnspecified
		}
		return ScopeReserved
	}

	// 0.0.0.0/32
	if ip.Equal(net.IPv4zero) {
		return ScopeUnspecified
	}

	// 10.0.0.0/8 - Private (RFC 1918)
	if b0 == 10 {
		return ScopePrivate
	}

	// 100.64.0.0/10 - CGNAT (RFC 6598)
	if b0 == 100 && b1 >= 64 && b1 <= 127 {
		return ScopeCGNAT
	}

	// 127.0.0.0/8 - Loopback
	if b0 == 127 {
		return ScopeLoopback
	}

	// 169.254.0.0/16 - Link-Local (APIPA)
	if b0 == 169 && b1 == 254 {
		return ScopeLinkLocal
	}

	// 172.16.0.0/12 - Private (RFC 1918)
	if b0 == 172 && b1 >= 16 && b1 <= 31 {
		return ScopePrivate
	}

	// 192.0.0.0/24 - IETF Protocol Assignments
	if b0 == 192 && b1 == 0 && ip[2] == 0 {
		return ScopeReserved
	}

	// 192.0.2.0/24 - Documentation (TEST-NET-1, RFC 5737)
	if b0 == 192 && b1 == 0 && ip[2] == 2 {
		return ScopeDocumentation
	}

	// 192.88.99.0/24 - 6to4 Relay
	if b0 == 192 && b1 == 88 && ip[2] == 99 {
		return ScopeReserved
	}

	// 192.168.0.0/16 - Private (RFC 1918)
	if b0 == 192 && b1 == 168 {
		return ScopePrivate
	}

	// 198.18.0.0/15 - Benchmarking (RFC 2544)
	if b0 == 198 && (b1 == 18 || b1 == 19) {
		return ScopeBenchmarking
	}

	// 198.51.100.0/24 - Documentation (TEST-NET-2, RFC 5737)
	if b0 == 198 && b1 == 51 && ip[2] == 100 {
		return ScopeDocumentation
	}

	// 203.0.113.0/24 - Documentation (TEST-NET-3, RFC 5737)
	if b0 == 203 && b1 == 0 && ip[2] == 113 {
		return ScopeDocumentation
	}

	// 224.0.0.0/4 - Multicast
	if b0 >= 224 && b0 <= 239 {
		return ScopeMulticast
	}

	// 240.0.0.0/4 - Reserved (former Class E)
	if b0 >= 240 && b0 <= 254 {
		return ScopeReserved
	}

	// 255.255.255.255/32 - Limited Broadcast
	if ip.Equal(net.IPv4bcast) {
		return ScopeBroadcast
	}

	return ScopePublic
}

func classifyIPv6Scope(ip net.IP) IPScope {
	if ip == nil || len(ip) < 16 {
		return ScopeUnknown
	}

	// ::1/128 - Loopback
	if ip.Equal(net.IPv6loopback) {
		return ScopeLoopback
	}

	// ::/128 - Unspecified
	if ip.Equal(net.IPv6zero) {
		return ScopeUnspecified
	}

	// ::ffff:0:0/96 - IPv4-mapped
	if strings.HasPrefix(ip.String(), "::ffff:") {
		ip4 := net.IPv4(ip[12], ip[13], ip[14], ip[15])
		return classifyIPv4Scope(ip4)
	}

	// fe80::/10 - Link-Local
	if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
		return ScopeLinkLocal
	}

	// fc00::/7 - Unique Local (ULA)
	if ip[0] == 0xfc || ip[0] == 0xfd {
		return ScopeUniqueLocal
	}

	// ff00::/8 - Multicast
	if ip[0] == 0xff {
		return ScopeMulticast
	}

	// 2001:db8::/32 - Documentation
	if ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
		return ScopeDocumentation
	}

	// 2001::/32 - Teredo / 2002::/16 - 6to4
	if ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x00 && ip[3] == 0x00 {
		return ScopeReserved
	}
	if ip[0] == 0x20 && ip[1] == 0x02 {
		return ScopeReserved
	}

	// 0100::/64 - Discard (RFC 6666)
	if ip[0] == 0x01 && ip[1] == 0x00 && ip[2] == 0x00 && ip[3] == 0x00 &&
		ip[4] == 0x00 && ip[5] == 0x00 && ip[6] == 0x00 && ip[7] == 0x00 {
		return ScopeDiscard
	}

	return ScopePublic
}

func ipScopeIsSuspicious(scope IPScope) bool {
	switch scope {
	case ScopeMulticast, ScopeBroadcast, ScopeReserved, ScopeDiscard, ScopeBenchmarking:
		return true
	default:
		return false
	}
}

func ipScopeRiskScore(scope IPScope) float64 {
	switch scope {
	case ScopeLoopback:
		return 0.0
	case ScopePrivate, ScopeLinkLocal, ScopeUniqueLocal:
		return 0.05
	case ScopeCGNAT:
		return 0.15
	case ScopeDocumentation, ScopeBenchmarking:
		return 0.85
	case ScopeMulticast, ScopeBroadcast:
		return 0.70
	case ScopeReserved, ScopeDiscard:
		return 0.90
	case ScopeIPv4Mapped:
		return 0.30
	case ScopePublic:
		return 0.10
	default:
		return 0.40
	}
}
