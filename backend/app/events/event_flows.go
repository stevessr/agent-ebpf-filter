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
	n := 0
	for i := range 4 {
		if i > 0 {
			buf[n] = '.'
			n++
		}
		octet := byte(addr >> (8 * i))
		if octet >= 100 {
			buf[n] = '0' + octet/100
			buf[n+1] = '0' + (octet/10)%10
			buf[n+2] = '0' + octet%10
			n += 3
		} else if octet >= 10 {
			buf[n] = '0' + octet/10
			buf[n+1] = '0' + octet%10
			n += 2
		} else {
			buf[n] = '0' + octet
			n++
		}
	}
	return string(buf[:n])
}

func NetParseIPForFlow(ip string) net.IP {
	if ip == "local" {
		return net.ParseIP("127.0.0.1")
	}
	return net.ParseIP(ip)
}

func RecordUDPFlowFromEvent(event *BpfEvent, out *pb.Event) {
	if out == nil {
		return
	}
	remoteIP := NetworkIP(event.NetFamily, event.NetAddr)
	if remoteIP == nil {
		return
	}
	remote := remoteIP.String()
	if remote == "" || remote == "<nil>" {
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
	out.IpScope = string(network.ClassifyIPScope(NetParseIPForFlow(dstIP)))
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
