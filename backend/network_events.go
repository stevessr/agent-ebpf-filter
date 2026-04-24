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
	default:
		return "unknown"
	}
}

func isNetworkEventType(eventType string) bool {
	switch eventType {
	case "network_connect", "network_bind", "network_sendto", "network_recvfrom":
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
	typeName := kernelEventTypeName(event.Type)

	out := &pb.Event{
		Pid:  event.PID,
		Ppid: event.PPID,
		Uid:  event.UID,
		Type: typeName,
		Tag:  getTagName(event.TagID),
		Comm: comm,
		Path: path,
	}

	if isNetworkEventType(typeName) {
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
