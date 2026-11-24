package events

import (
	"agent-ebpf-filter/app/captureprofile"
	"agent-ebpf-filter/pb"
	"fmt"
)

func BuildKernelEvent(event BpfEvent) *pb.Event {
	return BuildKernelEventFromRaw(&event)
}

func BuildKernelEventFromRaw(event *BpfEvent) *pb.Event {
	if event == nil {
		return nil
	}
	comm := SanitizeUTF8(event.Comm[:])
	path := SanitizeUTF8(event.Path[:])
	extraPath := SanitizeUTF8(event.Extra4[:])
	typeName := KernelEventTypeName(event.Type)

	out := &pb.Event{
		Pid:           event.PID,
		Ppid:          event.PPID,
		Uid:           event.UID,
		Gid:           event.GID,
		Tgid:          event.TGID,
		Type:          typeName,
		EventType:     pb.EventType(event.Type),
		Tag:           Deps.GetTagName(event.TagID),
		Comm:          comm,
		Path:          path,
		Retval:        event.Retval,
		DurationNs:    event.DurationNs,
		CgroupId:      event.CgroupID,
		ExtraPath:     extraPath,
		SchemaVersion: EventSchemaVersion,
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
		out.Domain = NetworkFamilyLabel(event.Extra1)
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
		name := Deps.SyscallName(event.Extra1)
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
		saddr := FormatIPv4Addr(event.Extra2)
		daddr := FormatIPv4Addr(uint32(event.Extra3))
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("%s:%d", daddr, event.NetPort)
		out.NetBytes = event.NetBytes
		out.ExtraInfo = fmt.Sprintf("saddr=%s sport=%d dport=%d", saddr, event.Extra1, event.NetPort)
	case "tcp_close":
		daddr := FormatIPv4Addr(uint32(event.Extra3))
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("%s:%d", daddr, event.NetPort)
		out.ExtraInfo = fmt.Sprintf("sport=%d dport=%d", event.Extra1, event.NetPort)
	case "tcp_state_change":
		oldState := uint8(event.DurationNs >> 32)
		newState := uint8(event.DurationNs & 0xFFFFFFFF)
		daddr := FormatIPv4Addr(uint32(event.Extra3))
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("%s:%d", daddr, event.NetPort)
		out.ExtraInfo = fmt.Sprintf("%s->%s sport=%d dport=%d",
			TCPStateName(oldState), TCPStateName(newState), event.Extra1, event.NetPort)
	case "dns_query":
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("dns:%d", event.NetPort)
		out.Domain = path
	case "socket_http":
		startLine, ok := captureprofile.ParseHTTP1StartLine(extraPath)
		out.CaptureSource = "kernel_socket_prefix"
		out.AppProtocol = "http1"
		out.KernelSocketFd = int32(event.Extra1)
		out.KernelPayloadPrefixLen = event.Extra2
		out.KernelCaptureFlags = event.KernelCaptureFlags
		out.Bytes = event.Extra3
		direction := "outgoing"
		if event.KernelCaptureFlags&kernelCaptureIncoming != 0 {
			direction = "incoming"
		}
		transport := ""
		switch event.KernelSocketType {
		case 1:
			transport = "tcp"
			out.SockType = "SOCK_STREAM"
		case 2:
			transport = "udp"
			out.SockType = "SOCK_DGRAM"
		case 3:
			out.SockType = "SOCK_RAW"
		}
		remoteIP := ""
		if addr := NetworkIP(event.NetFamily, event.NetAddr[:]); addr != nil {
			remoteIP = addr.String()
		}
		if ok && startLine.Kind == "request" {
			out.HttpMethod = startLine.Method
			out.HttpPath = startLine.Path
			out.ExtraPath = startLine.Method + " " + startLine.Path
			if match, matched := captureprofile.Default.MatchCompact(captureprofile.Observation{
				Source: "kernel_socket_prefix", Protocol: "http1", Direction: direction,
				Method: startLine.Method, Path: startLine.Path, Transport: transport,
				Family: NetworkFamilyLabel(event.NetFamily), RemoteIP: remoteIP, RemotePort: event.NetPort, Process: comm,
			}); matched {
				out.ApiProfile = match.ProfileID
				out.ApiVendor = match.Vendor
				out.ApiProduct = match.Product
				out.ApiOperation = match.Operation
				out.ApiConfidence = match.Confidence
				out.ServiceName = match.Vendor
			}
		} else if ok && startLine.Kind == "response" {
			out.HttpStatus = startLine.Status
			out.ExtraPath = fmt.Sprintf("HTTP status %d", startLine.Status)
		}
		kind := "start-line"
		if event.KernelCaptureFlags&kernelCaptureHTTPRequest != 0 {
			kind = "request-line"
		} else if event.KernelCaptureFlags&kernelCaptureHTTPResponse != 0 {
			kind = "response-line"
		}
		out.ExtraInfo = fmt.Sprintf("fd=%d capture=%s direction=%s prefix_len=%d bytes=%d flags=0x%x", int32(event.Extra1), kind, direction, event.Extra2, event.Extra3, event.KernelCaptureFlags)
	default:
		if event.Retval != 0 {
			out.ExtraInfo = fmt.Sprintf("retval=%d", event.Retval)
		}
	}

	var netSummary, netEndpoint, netRemote string
	if typeName == "accept" || typeName == "accept4" || IsNetworkEventType(typeName) {
		direction := NetworkDirectionLabel(event.NetDirection)
		netSummary, netEndpoint, netRemote = formatNetworkPathAndEndpoint(direction, event.NetFamily, event.NetAddr[:], event.NetPort, event.NetBytes)
		family := NetworkFamilyLabel(event.NetFamily)
		if netSummary != "" {
			out.Path = netSummary
		}
		out.NetDirection = direction
		out.NetEndpoint = netEndpoint
		out.NetBytes = event.NetBytes
		out.NetFamily = family
	}

	// Record TCP state and flow for network events
	if IsNetworkEventType(typeName) {
		srcPort := event.NetBytes
		dstPort := event.NetPort
		srcIP, dstIP := "", ""
		switch typeName {
		case "network_sendto", "network_recvfrom":
			// The endpoint pair is unusable; RecordUDPFlowFromEvent
			// populates the UDP flow fields with its own addresses.
			srcIP, dstIP = "0.0.0.0", "0.0.0.0"
		case "network_connect":
			if event.NetFamily == 2 {
				// NetFamily 2 carries the IPv4 destination in NetAddr[:4] in
				// host byte order; reuse the stack formatter instead of a
				// second net.IP.String allocation. Other families keep the
				// net path.
				dstIP = FormatIPv4Addr(binaryHostOrder(event.NetAddr))
			} else {
				dstIP = FormatIPv4Addr(uint32(event.Extra3))
				if addr := NetworkIP(event.NetFamily, event.NetAddr[:]); addr != nil {
					if s := addr.String(); s != "" && s != "<nil>" {
						dstIP = s
					}
				}
			}
			srcIP = "local"
			srcPort = 0
		default:
			srcIP = FormatIPv4Addr(event.Extra2)
			dstIP = FormatIPv4Addr(uint32(event.Extra3))
		}
		if srcIP != "0.0.0.0" && dstIP != "0.0.0.0" && dstPort > 0 {
			Deps.ApplyBestEffortProcessContextToEvent(out)
			flowState := ""
			switch typeName {
			case "network_connect":
				if event.Retval == 0 {
					flowState = "ESTABLISHED"
				} else if event.Retval < 0 {
					flowState = "FAILED"
				}
			case "tcp_close":
				flowState = "CLOSED"
			case "tcp_state_change":
				flowState = TCPStateName(uint8(event.DurationNs & 0xFFFFFFFF))
			}
			PopulateEventFlowFields(out, srcIP, dstIP, srcPort, dstPort, "TCP")
			Deps.Network.RecordFlowContext(srcIP, dstIP, srcPort, dstPort, out, flowState)
			Deps.Network.RecordBandwidthBytes(srcIP, dstIP, dstPort, "TCP", out.NetDirection, uint64(out.NetBytes), out.Comm, out.Pid)
			// Protocol detection reads the captured payload in place; the raw
			// bytes matter because TLS/DNS headers legitimately contain NULs.
			if payload := TrimNUL(event.Extra4[:]); len(payload) > 4 {
				entry := Deps.Network.DetectAndRecordProtocol(dstIP, dstPort, payload)
				Deps.Network.ApplyFlowProtocolMetadata(srcIP, dstIP, srcPort, dstPort, "TCP", entry)
				ApplyProtocolMetadataToEvent(out, entry)
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
		case "network_connect":
			if srcIP != "0.0.0.0" && dstIP != "0.0.0.0" && dstPort > 0 {
				Deps.Network.RecordTCPConnect(srcIP, dstIP, srcPort, dstPort, out.Pid, out.Comm)
				if event.Retval == 0 {
					Deps.Network.RecordTCPStateChange(srcIP, dstIP, srcPort, dstPort, uint8(TCPStateSynSent), uint8(TCPStateEstablished), out.Pid, out.Comm)
				}
			}
		case "tcp_connect":
			Deps.Network.RecordTCPConnect(srcIP, dstIP, srcPort, dstPort, out.Pid, out.Comm)
		case "tcp_close":
			Deps.Network.RecordTCPClose(srcIP, dstIP, srcPort, dstPort)
		case "tcp_state_change":
			oldState := uint8(event.DurationNs >> 32)
			newState := uint8(event.DurationNs & 0xFFFFFFFF)
			Deps.Network.RecordTCPStateChange(srcIP, dstIP, srcPort, dstPort, oldState, newState, out.Pid, out.Comm)
		}
		if (typeName == "network_sendto" || typeName == "network_recvfrom") && dstPort > 0 {
			RecordUDPFlowFromEvent(event, out, netRemote)
		}
	}

	Deps.KernelRisk(event, out)
	return out
}
