package main

import (
	"fmt"
	"net"

	"agent-ebpf-filter/pb"
)

func recordUDPFlowFromEvent(event bpfEvent, out *pb.Event) {
	if out == nil {
		return
	}
	remoteIP := networkIP(event.NetFamily, event.NetAddr)
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
	recordNetworkFlowContextFromEvent(srcIP, dstIP, srcPort, dstPort, out, "")
	populateEventFlowFields(out, srcIP, dstIP, srcPort, dstPort, "UDP")
	if extraPath := sanitizeUTF8(event.Extra4[:]); len(extraPath) > 4 {
		entry := detectAndRecordProtocol(remote, dstPort, []byte(extraPath))
		networkFlowAggregator.ApplyProtocolMetadata(srcIP, dstIP, srcPort, dstPort, "UDP", entry)
		applyProtocolMetadataToEvent(out, entry)
	}
}

func populateEventFlowFields(out *pb.Event, srcIP, dstIP string, srcPort, dstPort uint32, transport string) {
	if out == nil {
		return
	}
	key := makeFlowKey(srcIP, dstIP, srcPort, dstPort, transport)
	out.FlowId = key.ID()
	out.SrcIp = srcIP
	out.SrcPort = srcPort
	out.DstIp = dstIP
	out.DstPort = dstPort
	out.Transport = transport
	out.ServiceName = lookupServiceByPort(dstPort)
	out.IpScope = string(classifyIPScope(netParseIPForFlow(dstIP)))
	if domain, ok := dnsCorrelation.LookupIP(dstIP); ok {
		out.DnsName = domain
		if out.Domain == "" {
			out.Domain = domain
		}
	}
	out.AppProtocol = detectAppProtocol(dstPort, out.Domain)
	if out.NetDirection == "incoming" {
		out.BytesIn = uint64(out.NetBytes)
		out.PacketsIn = 1
	} else if out.NetDirection == "outgoing" {
		out.BytesOut = uint64(out.NetBytes)
		out.PacketsOut = 1
	}
}

func applyProtocolMetadataToEvent(out *pb.Event, entry *protoDetectionEntry) {
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

func netParseIPForFlow(ip string) net.IP {
	if ip == "local" {
		return net.ParseIP("127.0.0.1")
	}
	return net.ParseIP(ip)
}

func formatIPv4Addr(addr uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		addr&0xFF, (addr>>8)&0xFF, (addr>>16)&0xFF, (addr>>24)&0xFF)
}

var tcpStateNames = map[uint8]string{
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

func tcpStateName(state uint8) string {
	if name, ok := tcpStateNames[state]; ok {
		return name
	}
	return fmt.Sprintf("STATE_%d", state)
}
