package events

import (
	"bytes"
	"net"
	"strconv"
	"unicode/utf8"
	"unsafe"
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

// formatNetworkPathAndEndpoint builds the human-readable network summary and
// the "host:port" endpoint with one allocation: the endpoint and the remote
// host are substring views of the summary buffer, so the trio costs a single
// string. IPv6 hosts keep net.JoinHostPort's bracketing; IPv4 hosts never
// contain a colon.
func formatNetworkPathAndEndpoint(direction string, family uint32, addr []byte, port, nbytes uint32) (summary, endpoint, remote string) {
	// Worst case: "listening " + "[45-char IPv6]:65535" + " " + "(4294967295 B)".
	var buf [80]byte
	b := buf[:0]
	if direction != "" {
		b = append(b, direction...)
	}
	if family == 2 && len(addr) >= 4 {
		// Fast path: dotted quad straight from the sample buffer, no net.IP
		// intermediate.
		if len(b) > 0 {
			b = append(b, ' ')
		}
		remoteStart := len(b)
		b = appendIPv4Addr(b, uint32(addr[0])|uint32(addr[1])<<8|uint32(addr[2])<<16|uint32(addr[3])<<24)
		remoteEnd := len(b)
		if port != 0 {
			b = append(b, ':')
			b = strconv.AppendUint(b, uint64(port), 10)
		}
		summary = ownedString(b)
		return summary, summary[remoteStart:], summary[remoteStart:remoteEnd]
	}
	ip := NetworkIP(family, addr)
	if ip == nil {
		// No endpoint: the summary is just the direction and byte count.
		if nbytes > 0 {
			if len(b) > 0 {
				b = append(b, ' ')
			}
			b = append(b, '(')
			b = strconv.AppendUint(b, uint64(nbytes), 10)
			b = append(b, ' ', 'B', ')')
		}
		if len(b) == 0 {
			return "", "", ""
		}
		return ownedString(b), "", ""
	}
	host := ip.String()
	if len(b) > 0 {
		b = append(b, ' ')
	}
	remoteStart := len(b)
	if len(ip) == 16 {
		b = append(b, '[')
	}
	b = append(b, host...)
	remoteEnd := len(b)
	if len(ip) == 16 {
		b = append(b, ']')
	}
	if port != 0 {
		b = append(b, ':')
		b = strconv.AppendUint(b, uint64(port), 10)
	}
	summary = ownedString(b)
	return summary, summary[remoteStart:], summary[remoteStart:remoteEnd]
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
