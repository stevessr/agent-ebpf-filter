package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"agent-ebpf-filter/pb"
)

func kernelEventTypeName(eventType uint32) string {
	switch eventType {
	case 0:
		return "execve"
	case 1:
		return "openat"
	case 2:
		return "network_connect"
	case 3:
		return "mkdir"
	case 4:
		return "unlink"
	case 5:
		return "ioctl"
	case 6:
		return "network_bind"
	case 7:
		return "network_sendto"
	case 8:
		return "network_recvfrom"
	case 9:
		return "read"
	case 10:
		return "write"
	case 11:
		return "open"
	case 12:
		return "chmod"
	case 13:
		return "chown"
	case 14:
		return "rename"
	case 15:
		return "link"
	case 16:
		return "symlink"
	case 17:
		return "mknod"
	case 18:
		return "clone"
	case 19:
		return "exit"
	case 20:
		return "socket"
	case 21:
		return "accept"
	case 22:
		return "accept4"
	case 25:
		return "syscall"
	case 26:
		return "process_fork"
	case 27:
		return "process_exec"
	case 28:
		return "process_exit"
	case 29:
		return "wait4"
	case 30:
		return "semantic_alert"
	case 31:
		return "tcp_connect"
	case 32:
		return "tcp_close"
	case 33:
		return "tcp_state_change"
	case 34:
		return "dns_query"
	default:
		return "unknown"
	}
}

func isNetworkEventType(eventType string) bool {
	switch eventType {
	case "network_connect", "network_bind", "network_sendto", "network_recvfrom",
		"accept", "accept4", "socket",
		"tcp_connect", "tcp_close", "tcp_state_change", "dns_query":
		return true
	default:
		return false
	}
}

func networkDirectionLabel(direction uint32) string {
	switch direction {
	case 1:
		return "outgoing"
	case 2:
		return "incoming"
	case 3:
		return "listening"
	default:
		return ""
	}
}

func networkFamilyLabel(family uint32) string {
	switch family {
	case 2:
		return "ipv4"
	case 10:
		return "ipv6"
	default:
		return ""
	}
}

func networkIP(family uint32, addr [16]byte) net.IP {
	switch family {
	case 2:
		return net.IP(addr[:4]).To4()
	case 10:
		return net.IP(addr[:]).To16()
	default:
		return nil
	}
}

func formatNetworkEndpoint(family uint32, addr [16]byte, port uint32) string {
	ip := networkIP(family, addr)
	if ip == nil {
		return ""
	}

	host := ip.String()
	if port == 0 {
		return host
	}
	return net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10))
}

func formatNetworkSummary(direction, endpoint string, bytes uint32) string {
	if endpoint == "" && bytes == 0 {
		return ""
	}

	parts := make([]string, 0, 3)
	if direction != "" {
		parts = append(parts, direction)
	}
	if endpoint != "" {
		parts = append(parts, endpoint)
	}
	if bytes > 0 {
		parts = append(parts, fmt.Sprintf("(%d B)", bytes))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// sanitizeUTF8 converts a raw byte slice from the kernel to a valid UTF-8 string,
// replacing any invalid bytes with the Unicode replacement character.
func sanitizeUTF8(b []byte) string {
	return strings.ToValidUTF8(strings.TrimRight(string(b), "\x00"), "�")
}

func buildKernelEvent(event bpfEvent) *pb.Event {
	comm := sanitizeUTF8(event.Comm[:])
	path := sanitizeUTF8(event.Path[:])
	extraPath := sanitizeUTF8(event.Extra4[:])
	typeName := kernelEventTypeName(event.Type)

	out := &pb.Event{
		Pid:           event.PID,
		Ppid:          event.PPID,
		Uid:           event.UID,
		Gid:           event.GID,
		Tgid:          event.TGID,
		Type:          typeName,
		EventType:     pb.EventType(event.Type),
		Tag:           getTagName(event.TagID),
		Comm:          comm,
		Path:          path,
		Retval:        event.Retval,
		DurationNs:    event.DurationNs,
		CgroupId:      event.CgroupID,
		ExtraPath:     extraPath,
		SchemaVersion: eventSchemaVersion,
	}

	// Populate type-specific fields
	switch typeName {
	case "read", "write":
		out.ExtraInfo = fmt.Sprintf("fd=%d count=%d", event.Extra1, event.Extra3)
		out.Bytes = event.Extra3
	case "open":
		out.ExtraInfo = fmt.Sprintf("flags=0x%x mode=0%o", event.Extra1, event.Extra2)
		out.Mode = fmt.Sprintf("0%o", event.Extra2)
	case "chmod":
		out.Mode = fmt.Sprintf("0%o", event.Extra2)
		out.ExtraInfo = fmt.Sprintf("mode=0%o", event.Extra2)
	case "chown":
		out.UidArg = event.Extra1
		out.GidArg = event.Extra2
		out.ExtraInfo = fmt.Sprintf("uid=%d gid=%d", event.Extra1, event.Extra2)
	case "rename":
		out.ExtraInfo = fmt.Sprintf("newpath=%s", extraPath)
	case "link", "symlink":
		out.ExtraInfo = fmt.Sprintf("target=%s", extraPath)
	case "mknod":
		out.Mode = fmt.Sprintf("0%o", event.Extra1)
		out.ExtraInfo = fmt.Sprintf("mode=0%o dev=0x%x", event.Extra1, event.Extra2)
	case "ioctl":
		out.ExtraInfo = fmt.Sprintf("request=0x%x", event.Extra1)
	case "clone":
		out.ExtraInfo = fmt.Sprintf("flags=0x%x", event.Extra1)
		if event.Retval > 0 {
			out.ExtraInfo += fmt.Sprintf(" child_pid=%d", event.Retval)
		}
	case "exit":
		out.ExtraInfo = fmt.Sprintf("status=%d", event.Extra1)
	case "process_fork":
		out.ExtraInfo = fmt.Sprintf("child_pid=%d", event.Extra1)
		if path == "" {
			out.Path = fmt.Sprintf("pid=%d", event.Extra1)
		}
	case "process_exec":
		out.ExtraInfo = fmt.Sprintf("old_pid=%d", event.Extra1)
	case "process_exit":
		out.ExtraInfo = fmt.Sprintf("group_dead=%t", event.Extra1 != 0)
	case "wait4":
		out.ExtraInfo = fmt.Sprintf("target_pid=%d options=0x%x", int32(event.Extra1), event.Extra2)
	case "socket":
		out.Domain = networkFamilyLabel(event.Extra1)
		if event.Extra1 == 1 {
			out.Domain = "unix"
		}
		switch event.Extra2 {
		case 1:
			out.SockType = "SOCK_STREAM"
		case 2:
			out.SockType = "SOCK_DGRAM"
		case 3:
			out.SockType = "SOCK_RAW"
		default:
			out.SockType = fmt.Sprintf("type=%d", event.Extra2)
		}
		out.Protocol = uint32(event.Extra3)
		out.ExtraInfo = fmt.Sprintf("domain=%s type=%s protocol=%d", out.Domain, out.SockType, out.Protocol)
	case "unlinkat":
		out.ExtraInfo = fmt.Sprintf("flags=0x%x", event.Extra1)
	case "mkdirat":
		out.Mode = fmt.Sprintf("0%o", event.Extra1)
		out.ExtraInfo = fmt.Sprintf("mode=0%o", event.Extra1)
	case "syscall":
		name := syscallName(event.Extra1)
		if name != "" {
			out.ExtraInfo = fmt.Sprintf("%s(%d)", name, event.Extra1)
		} else {
			out.ExtraInfo = fmt.Sprintf("nr=%d", event.Extra1)
		}
		if event.Extra2 != 0 {
			out.ExtraInfo += fmt.Sprintf(" arg=%d", event.Extra2)
		}
		if event.Extra3 != 0 {
			out.ExtraInfo += fmt.Sprintf(" arg2=%d", event.Extra3)
		}
		if event.Retval < 0 {
			out.ExtraInfo += fmt.Sprintf(" err=%d", event.Retval)
		}
	case "tcp_connect":
		saddr := formatIPv4Addr(event.Extra2)
		daddr := formatIPv4Addr(uint32(event.Extra3))
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("%s:%d", daddr, event.NetPort)
		out.NetBytes = event.NetBytes
		out.ExtraInfo = fmt.Sprintf("saddr=%s sport=%d dport=%d", saddr, event.Extra1, event.NetPort)
	case "tcp_close":
		daddr := formatIPv4Addr(uint32(event.Extra3))
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("%s:%d", daddr, event.NetPort)
		out.ExtraInfo = fmt.Sprintf("sport=%d dport=%d", event.Extra1, event.NetPort)
	case "tcp_state_change":
		oldState := uint8(event.DurationNs >> 32)
		newState := uint8(event.DurationNs & 0xFFFFFFFF)
		daddr := formatIPv4Addr(uint32(event.Extra3))
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("%s:%d", daddr, event.NetPort)
		out.ExtraInfo = fmt.Sprintf("%s->%s sport=%d dport=%d",
			tcpStateName(oldState), tcpStateName(newState), event.Extra1, event.NetPort)
	case "dns_query":
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("dns:%d", event.NetPort)
		out.Domain = sanitizeUTF8(event.Path[:])
	default:
		if event.Retval != 0 {
			out.ExtraInfo = fmt.Sprintf("retval=%d", event.Retval)
		}
	}

	if typeName == "accept" || typeName == "accept4" || isNetworkEventType(typeName) {
		direction := networkDirectionLabel(event.NetDirection)
		endpoint := formatNetworkEndpoint(event.NetFamily, event.NetAddr, event.NetPort)
		family := networkFamilyLabel(event.NetFamily)
		summary := formatNetworkSummary(direction, endpoint, event.NetBytes)
		if summary != "" {
			out.Path = summary
		}
		out.NetDirection = direction
		out.NetEndpoint = endpoint
		out.NetBytes = event.NetBytes
		out.NetFamily = family
	}

	// Record TCP state and flow for network events
	if isNetworkEventType(typeName) {
		srcIP := formatIPv4Addr(event.Extra2)
		dstIP := formatIPv4Addr(uint32(event.Extra3))
		srcPort := event.NetBytes
		dstPort := event.NetPort

		// Generic syscall tracepoints (network_connect, network_sendto,
		// network_recvfrom, etc.) store addresses in NetAddr, not
		// Extra2/Extra3. Extra3 contains byte counts for sendto/recvfrom
		// which would produce bogus flow keys.
		//
		// TCP flow tracepoints (tcp_connect, tcp_close, tcp_state_change)
		// pack addresses into Extra2/Extra3 via emit_tcp_flow_event.
		switch typeName {
		case "network_sendto", "network_recvfrom":
			// Handled by recordUDPFlowFromEvent below — skip the TCP path.
			srcIP, dstIP = "0.0.0.0", "0.0.0.0"
		case "network_connect":
			// Tagged processes get a full tcp_connect event with both
			// srcIP and dstIP from Extra2/Extra3.  Untagged processes
			// only see this generic event — extract dstIP from NetAddr.
			if event.TagID == 0 {
				if addr := networkIP(event.NetFamily, event.NetAddr); addr != nil {
					if s := addr.String(); s != "" && s != "<nil>" {
						dstIP = s
					}
				}
			} else {
				srcIP, dstIP = "0.0.0.0", "0.0.0.0"
			}
		}

		if srcIP != "0.0.0.0" && dstIP != "0.0.0.0" && dstPort > 0 {
			applyBestEffortProcessContextToEvent(out)
			flowState := ""
			switch typeName {
			case "tcp_close":
				flowState = "CLOSED"
			case "tcp_state_change":
				flowState = tcpStateName(uint8(event.DurationNs & 0xFFFFFFFF))
			}
			populateEventFlowFields(out, srcIP, dstIP, srcPort, dstPort, "TCP")
			recordNetworkFlowContextFromEvent(srcIP, dstIP, srcPort, dstPort, out, flowState)
			globalBandwidthTracker.RecordBytes(srcIP, dstIP, dstPort, "TCP", out.NetDirection, uint64(out.NetBytes), out.Comm, out.Pid)
			// Protocol detection from captured payload
			if extraPath := sanitizeUTF8(event.Extra4[:]); len(extraPath) > 4 {
				entry := detectAndRecordProtocol(dstIP, dstPort, []byte(extraPath))
				networkFlowAggregator.ApplyProtocolMetadata(srcIP, dstIP, srcPort, dstPort, "TCP", entry)
				applyProtocolMetadataToEvent(out, entry)
				if entry != nil && entry.SNI != "" {
					out.Domain = entry.SNI
					out.NetEndpoint = fmt.Sprintf("%s:%d [SNI: %s]", dstIP, dstPort, entry.SNI)
				}
				if entry != nil && entry.HTTPHost != "" {
					out.Domain = entry.HTTPHost
					out.NetEndpoint = fmt.Sprintf("%s:%d [Host: %s]", dstIP, dstPort, entry.HTTPHost)
				}
			}
		}
		// TCP state tracking
		switch typeName {
		case "tcp_connect":
			tcpTracker.RecordConnect(srcIP, dstIP, srcPort, dstPort, out.Pid, out.Comm)
		case "tcp_close":
			tcpTracker.RecordClose(srcIP, dstIP, srcPort, dstPort)
		case "tcp_state_change":
			oldState := uint8(event.DurationNs >> 32)
			newState := uint8(event.DurationNs & 0xFFFFFFFF)
			tcpTracker.RecordStateChange(srcIP, dstIP, srcPort, dstPort, oldState, newState, out.Pid, out.Comm)
		}
		if (typeName == "network_sendto" || typeName == "network_recvfrom") && dstPort > 0 {
			recordUDPFlowFromEvent(event, out)
		}
	}

	return out
}

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
