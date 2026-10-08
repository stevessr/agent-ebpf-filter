package events

import (
	"fmt"
	"net"

	"agent-ebpf-filter/internal/network"
	"agent-ebpf-filter/pb"
)

// ── Flow-related helpers ──────────────────────────────────────────────

var TCPStateNames = map[uint8]string{
	1:  "ESTABLISHED",
	2:  "SYN_SENT",
	3:  "SYN_RECV",
	4:  "FIN_WAIT1",
	5:  "FIN_WAIT2",
	6:  "TIME_WAIT",
	7:  "CLOSE",
	8:  "CLOSE_WAIT",
	9:  "LAST_ACK",
	10: "LISTEN",
	11: "CLOSING",
}

func TCPStateName(state uint8) string {
	if name, ok := TCPStateNames[state]; ok {
		return name
	}
	return fmt.Sprintf("STATE_%d", state)
}

// FormatIPv4Addr renders a host-byte-order IPv4 address as dotted quad. It
// writes the digits into a stack buffer and allocates the final string once
// instead of going through fmt.
func FormatIPv4Addr(addr uint32) string {
	var buf [15]byte // "255.255.255.255"
	return string(appendIPv4Addr(buf[:0], addr))
}

// appendIPv4Addr appends the dotted-quad form of a host-byte-order IPv4
// address to b, letting hot-path callers assemble host:port strings with a
// single allocation.
func appendIPv4Addr(b []byte, addr uint32) []byte {
	for i := range 4 {
		if i > 0 {
			b = append(b, '.')
		}
		octet := byte(addr >> (8 * i))
		switch {
		case octet >= 100:
			b = append(b, '0'+octet/100, '0'+(octet/10)%10, '0'+octet%10)
		case octet >= 10:
			b = append(b, '0'+octet/10, '0'+octet%10)
		default:
			b = append(b, '0'+octet)
		}
	}
	return b
}

func NetParseIPForFlow(ip string) net.IP {
	if ip == "local" {
		return net.ParseIP("127.0.0.1")
	}
	return net.ParseIP(ip)
}

func RecordUDPFlowFromEvent(event *BpfEvent, out *pb.Event, remote string) {
	if out == nil || remote == "" {
		return
	}
	srcIP, dstIP := "local", remote
	srcPort, dstPort := uint32(0), event.NetPort
	if out.GetNetDirection() == "incoming" {
		srcIP, dstIP = remote, "local"
	}
	Deps.Network.RecordFlowContext(srcIP, dstIP, srcPort, dstPort, out, "")
	PopulateEventFlowFields(out, srcIP, dstIP, srcPort, dstPort, "UDP")
	if payload := TrimNUL(event.Extra4[:]); len(payload) > 4 {
		entry := Deps.Network.DetectAndRecordProtocol(remote, dstPort, payload)
		Deps.Network.ApplyFlowProtocolMetadata(srcIP, dstIP, srcPort, dstPort, "UDP", entry)
		ApplyProtocolMetadataToEvent(out, entry)
	}
}

func PopulateEventFlowFields(out *pb.Event, srcIP, dstIP string, srcPort, dstPort uint32, transport string) {
	if out == nil {
		return
	}
	key := network.MakeFlowKey(srcIP, dstIP, srcPort, dstPort, transport)
	out.FlowId = key.ID()
	out.SrcIp = srcIP
	out.SrcPort = srcPort
	out.DstIp = dstIP
	out.DstPort = dstPort
	out.Transport = transport
	out.ServiceName = network.LookupServiceByPort(dstPort)
	out.IpScope = string(classifyFlowIPScope(dstIP))
	if domain, ok := Deps.Network.LookupDNS(dstIP); ok {
		out.DnsName = domain
		if out.Domain == "" {
			out.Domain = domain
		}
	}
	out.AppProtocol = network.DetectAppProtocol(dstPort, out.Domain)
	if out.NetDirection == "incoming" {
		out.BytesIn = uint64(out.NetBytes)
		out.PacketsIn = 1
	} else if out.NetDirection == "outgoing" {
		out.BytesOut = uint64(out.NetBytes)
		out.PacketsOut = 1
	}
}

func ApplyProtocolMetadataToEvent(out *pb.Event, entry *ProtoDetectionEntry) {
	if out == nil || entry == nil {
		return
	}
	out.AppProtocol = string(entry.AppProtocol)
	if entry.SNI != "" {
		out.Sni = entry.SNI
		if out.Domain == "" {
			out.Domain = entry.SNI
		}
	}
	if entry.ALPN != "" {
		out.TlsAlpn = entry.ALPN
	}
	if entry.HTTPHost != "" {
		out.HttpHost = entry.HTTPHost
		if out.Domain == "" || out.Domain == out.Sni {
			out.Domain = entry.HTTPHost
		}
		if entry.AppProtocol == AppProtoDNS || entry.AppProtocol == AppProtomDNS {
			out.DnsName = entry.HTTPHost
		}
	}
}

// classifyFlowIPScope classifies dstIP for the flow record without
// net.ParseIP: dotted quads decode straight from the string, "local" maps to
// loopback, and anything else (IPv6 or unparseable) falls back to the net
// path so the recorded scope never diverges from ClassifyIPScope.
func classifyFlowIPScope(dstIP string) network.IPScope {
	if dstIP == "local" {
		return network.ScopeLoopback
	}
	var octets [4]byte
	if ipv4OctetsFromString(dstIP, &octets) {
		return network.ClassifyIPv4ScopeBytes(octets[0], octets[1], octets[2], octets[3])
	}
	return network.ClassifyIPScope(NetParseIPForFlow(dstIP))
}

// ipv4OctetsFromString decodes a strict a.b.c.d (each 0-255, no leading
// zeros) into out. It reports false for anything else, including IPv6.
func ipv4OctetsFromString(s string, out *[4]byte) bool {
	fields := 0
	octet := 0
	digits := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			if digits == 1 && octet == 0 {
				return false // leading zero
			}
			octet = octet*10 + int(c-'0')
			digits++
			if digits > 3 || octet > 255 {
				return false
			}
		case c == '.':
			if digits == 0 || fields == 3 {
				return false
			}
			out[fields] = byte(octet)
			fields++
			octet = 0
			digits = 0
		default:
			return false
		}
	}
	if digits == 0 || fields != 3 {
		return false
	}
	out[3] = byte(octet)
	return true
}
