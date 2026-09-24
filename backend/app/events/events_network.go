package events

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"unicode/utf8"
	"unsafe"

	"agent-ebpf-filter/app/captureprofile"
	"agent-ebpf-filter/pb"
)

// ── Pure helper functions (no external deps needed) ────────────────────

const (
	kernelCaptureHTTPRequest uint32 = 1 << iota
	kernelCaptureHTTPResponse
	kernelCaptureIncoming
	kernelCaptureOutgoing
	kernelCaptureFDDuplicated
	kernelCaptureFDInherited
	kernelCaptureAccepted
	kernelCaptureScatterGather
)

func KernelEventTypeName(eventType uint32) string {
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
	case 43:
		return "socket_http"
	default:
		return "unknown"
	}
}

func IsNetworkEventType(eventType string) bool {
	switch eventType {
	case "network_connect", "network_bind", "network_sendto", "network_recvfrom",
		"accept", "accept4", "socket",
		"tcp_connect", "tcp_close", "tcp_state_change", "dns_query", "socket_http":
		return true
	default:
		return false
	}
}

func NetworkDirectionLabel(direction uint32) string {
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

func NetworkFamilyLabel(family uint32) string {
	switch family {
	case 2:
		return "ipv4"
	case 10:
		return "ipv6"
	default:
		return ""
	}
}

// NetworkIP returns a view over addr for the given address family; it never
// copies. Callers must not retain or mutate the result beyond the lifetime of
// addr — on the hot path addr is the ring-buffer sample itself.
func NetworkIP(family uint32, addr []byte) net.IP {
	switch family {
	case 2:
		if len(addr) < 4 {
			return nil
		}
		return net.IP(addr[:4]).To4()
	case 10:
		if len(addr) < 16 {
			return nil
		}
		return net.IP(addr[:16]).To16()
	default:
		return nil
	}
}

// binaryHostOrder reads the first four bytes of a network address buffer as a
// little-endian uint32 the same way FormatIPv4Addr consumes host-order IPv4
// values carried in the Extra2/Extra3 fields.
func binaryHostOrder(addr [16]byte) uint32 {
	return uint32(addr[0]) | uint32(addr[1])<<8 | uint32(addr[2])<<16 | uint32(addr[3])<<24
}

// FormatNetworkEndpoint renders "host" or "host:port" for the family's
// address. IPv6 hosts keep net.JoinHostPort's bracketing; IPv4 hosts never
// contain a colon so the port is appended with one final allocation.
func FormatNetworkEndpoint(family uint32, addr []byte, port uint32) string {
	if family == 2 && len(addr) >= 4 {
		// Fast path: dotted quad straight from the sample buffer plus the
		// port, one allocation, no net.IP intermediate.
		var buf [15 + 1 + 5]byte // "255.255.255.255" + ':' + port
		b := appendIPv4Addr(buf[:0], uint32(addr[0])|uint32(addr[1])<<8|uint32(addr[2])<<16|uint32(addr[3])<<24)
		if port == 0 {
			return ownedString(b)
		}
		b = append(b, ':')
		b = strconv.AppendUint(b, uint64(port), 10)
		return ownedString(b)
	}
	ip := NetworkIP(family, addr)
	if ip == nil {
		return ""
	}
	host := ip.String()
	if port == 0 {
		return host
	}
	if len(ip) == 16 {
		return net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10))
	}
	var portBuf [5]byte
	portDigits := strconv.AppendUint(portBuf[:0], uint64(port), 10)
	endpoint := make([]byte, 0, len(host)+1+len(portDigits))
	endpoint = append(endpoint, host...)
	endpoint = append(endpoint, ':')
	endpoint = append(endpoint, portDigits...)
	return string(endpoint)
}

// FormatNetworkSummary joins "direction endpoint (N B)" with single spaces.
// It writes into a stack buffer and allocates the result once instead of
// building a parts slice.
func FormatNetworkSummary(direction, endpoint string, bytes uint32) string {
	if endpoint == "" && bytes == 0 {
		return ""
	}
	needSpace := false
	size := 0
	if direction != "" {
		size += len(direction)
		needSpace = true
	}
	if endpoint != "" {
		if needSpace {
			size++
		}
		size += len(endpoint)
		needSpace = true
	}
	if bytes > 0 {
		if needSpace {
			size++
		}
		size += len("(18446744073 B)") + 8 // uint32 decimal worst case fits comfortably
	}
	b := make([]byte, 0, size)
	if direction != "" {
		b = append(b, direction...)
		needSpace = true
	}
	if endpoint != "" {
		if needSpace {
			b = append(b, ' ')
		}
		b = append(b, endpoint...)
		needSpace = true
	}
	if bytes > 0 {
		if needSpace {
			b = append(b, ' ')
		}
		b = append(b, '(')
		b = strconv.AppendUint(b, uint64(bytes), 10)
		b = append(b, ' ', 'B', ')')
	}
	// Trimmed join previously collapsed stray spaces; inputs are fixed labels
	// so no trimming is needed.
	return ownedString(b)
}

// SanitizeUTF8 converts a raw byte slice from the kernel to a valid UTF-8 string,
// replacing any invalid bytes with the Unicode replacement character.
// eBPF tracepoints write paths into fixed-size buffers; trailing NUL padding is
// trimmed first, then any embedded NUL bytes (from uninitialised buffer regions
// or multi-field packing) are removed before the UTF‑8 safety pass.
//
// The bounds are computed on the byte view so only the populated prefix is
// ever copied out of the ring-buffer sample; valid input costs exactly one
// allocation of its own length.
func SanitizeUTF8(b []byte) string {
	b = TrimNUL(b)
	if len(b) == 0 {
		return ""
	}
	if bytes.IndexByte(b, 0) < 0 {
		if utf8.Valid(b) {
			return string(b)
		}
		return ownedString(bytes.ToValidUTF8(b, utf8ReplacementBytes))
	}
	cleaned := make([]byte, 0, len(b))
	for _, c := range b {
		if c != 0 {
			cleaned = append(cleaned, c)
		}
	}
	if !utf8.Valid(cleaned) {
		cleaned = bytes.ToValidUTF8(cleaned, utf8ReplacementBytes)
	}
	return ownedString(cleaned)
}

var utf8ReplacementBytes = []byte("�")

// ownedString views a freshly allocated, never-again-mutated byte slice as a
// string without copying it. Callers must not retain b.
func ownedString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// zeroPad is compared against buffer tails so a NUL-terminated string can be
// bounded with one SIMD search plus one memequal instead of a byte-by-byte
// backwards scan through the padding.
var zeroPad [1024]byte

// TrimNUL returns the populated prefix of a fixed-size kernel buffer as a view
// into b (no copy), exactly like bytes.TrimRight(b, "\x00"). The result is
// only valid while b is.
func TrimNUL(b []byte) []byte {
	i := bytes.IndexByte(b, 0)
	if i < 0 {
		return b
	}
	if tail := b[i+1:]; len(tail) <= len(zeroPad) && bytes.Equal(tail, zeroPad[:len(tail)]) {
		return b[:i]
	}
	return bytes.TrimRight(b, "\x00")
}

// ── Event builder ─────────────────────────────────────────────────────

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

	if typeName == "accept" || typeName == "accept4" || IsNetworkEventType(typeName) {
		direction := NetworkDirectionLabel(event.NetDirection)
		endpoint := FormatNetworkEndpoint(event.NetFamily, event.NetAddr[:], event.NetPort)
		family := NetworkFamilyLabel(event.NetFamily)
		summary := FormatNetworkSummary(direction, endpoint, event.NetBytes)
		if summary != "" {
			out.Path = summary
		}
		out.NetDirection = direction
		out.NetEndpoint = endpoint
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
			RecordUDPFlowFromEvent(event, out)
		}
	}

	Deps.KernelRisk(event, out)
	return out
}
