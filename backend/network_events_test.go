package main

import (
	"strings"
	"testing"

	"agent-ebpf-filter/pb"
)

func TestKernelEventTypeNameMatchesProtoNetworkEnums(t *testing.T) {
	tests := map[pb.EventType]string{
		pb.EventType_SEMANTIC_ALERT:   "semantic_alert",
		pb.EventType_TCP_CONNECT:      "tcp_connect",
		pb.EventType_TCP_CLOSE:        "tcp_close",
		pb.EventType_TCP_STATE_CHANGE: "tcp_state_change",
		pb.EventType_DNS_QUERY:        "dns_query",
	}
	for eventType, want := range tests {
		if got := kernelEventTypeName(uint32(eventType)); got != want {
			t.Fatalf("kernelEventTypeName(%d) = %q, want %q", eventType, got, want)
		}
	}
}

func TestFlowEventsAreNetworkEvents(t *testing.T) {
	for _, eventType := range []string{"tcp_connect", "tcp_close", "tcp_state_change", "dns_query"} {
		if !isNetworkEventType(eventType) {
			t.Fatalf("%s should be classified as a network event", eventType)
		}
	}
}

func TestBuildKernelEventRecordsUDPFlow(t *testing.T) {
	orig := networkFlowAggregator
	networkFlowAggregator = newFlowAggregator()
	defer func() { networkFlowAggregator = orig }()

	event := bpfEvent{
		PID:          42,
		Type:         uint32(pb.EventType_NETWORK_SENDTO),
		NetFamily:    2,
		NetDirection: 1,
		NetBytes:     29,
		NetPort:      53,
	}
	copy(event.Comm[:], []byte("dig"))
	copy(event.NetAddr[:4], []byte{8, 8, 8, 8})
	copy(event.Extra4[:], []byte{
		0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x03, 'c', 'o', 'm', 0x00,
		0x00, 0x01, 0x00, 0x01,
	})

	out := buildKernelEvent(event)
	if out == nil || out.NetEndpoint != "8.8.8.8:53" {
		t.Fatalf("NetEndpoint = %q, want 8.8.8.8:53", out.GetNetEndpoint())
	}
	if out.GetFlowId() != "UDP:local:0->8.8.8.8:53" || out.GetTransport() != "UDP" || out.GetDnsName() != "example.com" {
		t.Fatalf("proto flow fields = id %q transport %q dns %q", out.GetFlowId(), out.GetTransport(), out.GetDnsName())
	}
	result := networkFlowAggregator.Query(networkFlowQuery{ShowHistoric: true, Filter: "proto:udp host:example.com", Limit: 10})
	if result.Total != 1 || len(result.Flows) != 1 {
		t.Fatalf("UDP flow query total=%d len=%d, want 1", result.Total, len(result.Flows))
	}
	flow := result.Flows[0]
	if flow.Protocol != "UDP" || flow.DstIP != "8.8.8.8" || flow.DstPort != 53 || flow.DNSName != "example.com" {
		t.Fatalf("flow = protocol %q dst %s:%d dns %q", flow.Protocol, flow.DstIP, flow.DstPort, flow.DNSName)
	}
}

func TestBuildKernelEventCopiesDurationNs(t *testing.T) {
	event := bpfEvent{
		PID:        42,
		PPID:       7,
		UID:        1000,
		GID:        1001,
		Type:       25,
		TagID:      0,
		Retval:     -1,
		Extra1:     62,
		Extra2:     9,
		CgroupID:   123456,
		DurationNs: 987654321,
	}

	out := buildKernelEvent(event)
	if out == nil {
		t.Fatal("buildKernelEvent returned nil")
	}
	if out.DurationNs != event.DurationNs {
		t.Fatalf("DurationNs = %d, want %d", out.DurationNs, event.DurationNs)
	}
	if out.Gid != event.GID {
		t.Fatalf("Gid = %d, want %d", out.Gid, event.GID)
	}
	if out.CgroupId != event.CgroupID {
		t.Fatalf("CgroupId = %d, want %d", out.CgroupId, event.CgroupID)
	}
	if !strings.Contains(out.ExtraInfo, "kill(62)") {
		t.Fatalf("ExtraInfo = %q, want syscall name and number", out.ExtraInfo)
	}
}

func TestBuildKernelEventProcessFork(t *testing.T) {
	event := bpfEvent{
		PID:    100,
		PPID:   50,
		Type:   26,
		Extra1: 101,
	}
	copy(event.Path[:], []byte("python3"))

	out := buildKernelEvent(event)
	if out == nil {
		t.Fatal("buildKernelEvent returned nil")
	}
	if out.Type != "process_fork" {
		t.Fatalf("Type = %q, want process_fork", out.Type)
	}
	if !strings.Contains(out.ExtraInfo, "child_pid=101") {
		t.Fatalf("ExtraInfo = %q, want child pid", out.ExtraInfo)
	}
}
