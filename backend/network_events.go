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
	default:
		return "unknown"
	}
}

func isNetworkEventType(eventType string) bool {
	switch eventType {
	case "network_connect", "network_bind", "network_sendto", "network_recvfrom", "accept", "accept4":
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

func buildKernelEvent(event bpfEvent) *pb.Event {
	comm := strings.TrimRight(string(event.Comm[:]), "\x00")
	path := strings.TrimRight(string(event.Path[:]), "\x00")
	extraPath := strings.TrimRight(string(event.Extra4[:]), "\x00")
	typeName := kernelEventTypeName(event.Type)

	out := &pb.Event{
		Pid:       event.PID,
		Ppid:      event.PPID,
		Uid:       event.UID,
		Type:      typeName,
		EventType: pb.EventType(event.Type),
		Tag:       getTagName(event.TagID),
		Comm:      comm,
		Path:      path,
		Retval:    event.Retval,
		ExtraPath: extraPath,
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
	case "exit":
		out.ExtraInfo = fmt.Sprintf("status=%d", event.Extra1)
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

	return out
}
