package main

import (
	"strings"
	"testing"

	"agent-ebpf-filter/pb"
)

func TestFlowAggregatorKeysByFiveTupleAndKeepsAgentContext(t *testing.T) {
	agg := newFlowAggregator()
	ev := &pb.Event{
		Pid:        123,
		Comm:       "curl",
		NetBytes:   256,
		AgentRunId: "run-1",
		TaskId:     "task-1",
		ToolCallId: "tool-1",
		TraceId:    "trace-1",
		SpanId:     "span-1",
		Decision:   "ALLOW",
	}
	agg.RecordConnectionContext("10.0.0.2", "93.184.216.34", 40000, 443, "TCP", "curl", 123, "outgoing", "ESTABLISHED", ev)
	agg.RecordConnectionContext("10.0.0.2", "93.184.216.34", 40001, 443, "TCP", "curl", 123, "outgoing", "ESTABLISHED", ev)

	result := agg.Query(networkFlowQuery{ShowHistoric: true, Limit: 10})
	if result.Total != 2 {
		t.Fatalf("Total = %d, want 2 distinct source-port flows", result.Total)
	}
	for _, flow := range result.Flows {
		if flow.FlowID == "" {
			t.Fatal("FlowID should be populated")
		}
		if len(flow.AgentRunIDs) != 1 || flow.AgentRunIDs[0] != "run-1" {
			t.Fatalf("AgentRunIDs = %#v, want run-1", flow.AgentRunIDs)
		}
	}
}

func TestFlowQueryFilterSortCursorAndHistoric(t *testing.T) {
	agg := newFlowAggregator()
	agg.RecordConnectionContext("10.0.0.2", "93.184.216.34", 40000, 443, "TCP", "curl", 10, "outgoing", "ESTABLISHED", &pb.Event{Pid: 10, Comm: "curl", NetBytes: 100})
	agg.RecordConnectionContext("10.0.0.2", "192.0.2.10", 40001, 22, "TCP", "ssh", 11, "outgoing", "CLOSED", &pb.Event{Pid: 11, Comm: "ssh", NetBytes: 200})

	activeOnly := agg.Query(networkFlowQuery{Limit: 10})
	if activeOnly.Total != 1 {
		t.Fatalf("active-only Total = %d, want 1", activeOnly.Total)
	}
	all := agg.Query(networkFlowQuery{ShowHistoric: true, Filter: "process:ssh state:closed", Sort: "risk", Limit: 1})
	if all.Total != 1 || len(all.Flows) != 1 {
		t.Fatalf("historic filtered result = total %d len %d, want 1", all.Total, len(all.Flows))
	}
	if !all.Flows[0].Historic {
		t.Fatal("closed flow should be marked historic")
	}
	page := agg.Query(networkFlowQuery{ShowHistoric: true, Limit: 1})
	if page.Total != 2 || page.NextCursor == "" || len(page.Flows) != 1 {
		t.Fatalf("page = total %d next %q len %d, want cursor page", page.Total, page.NextCursor, len(page.Flows))
	}
}

func TestFlowAggregatorAppliesProtocolMetadata(t *testing.T) {
	agg := newFlowAggregator()
	agg.RecordConnectionContext("10.0.0.2", "93.184.216.34", 40000, 80, "TCP", "curl", 10, "outgoing", "ESTABLISHED", &pb.Event{Pid: 10, Comm: "curl", NetBytes: 128})
	agg.ApplyProtocolMetadata("10.0.0.2", "93.184.216.34", 40000, 80, "TCP", &protoDetectionEntry{
		AppProtocol: AppProtoHTTP,
		HTTPHost:    "example.com",
		HTTPMethod:  "GET",
	})

	result := agg.Query(networkFlowQuery{ShowHistoric: true, Filter: "host:example.com", Limit: 10})
	if result.Total != 1 || len(result.Flows) != 1 {
		t.Fatalf("filtered result = total %d len %d, want 1", result.Total, len(result.Flows))
	}
	flow := result.Flows[0]
	if flow.AppProtocol != "HTTP" || flow.HTTPHost != "example.com" || flow.HTTPMethod != "GET" || flow.DstDomain != "example.com" {
		t.Fatalf("flow protocol metadata = app=%q host=%q method=%q domain=%q", flow.AppProtocol, flow.HTTPHost, flow.HTTPMethod, flow.DstDomain)
	}
}

func TestFlowAggregatorAppliesTLSMetadata(t *testing.T) {
	agg := newFlowAggregator()
	agg.RecordConnectionContext("10.0.0.2", "93.184.216.34", 40000, 443, "TCP", "curl", 10, "outgoing", "ESTABLISHED", &pb.Event{Pid: 10, Comm: "curl", NetBytes: 128})
	agg.ApplyProtocolMetadata("10.0.0.2", "93.184.216.34", 40000, 443, "TCP", &protoDetectionEntry{
		AppProtocol: AppProtoTLS,
		SNI:         "api.example.com",
		ALPN:        "h2, http/1.1",
	})

	result := agg.Query(networkFlowQuery{ShowHistoric: true, Filter: "sni:api.example.com", Limit: 10})
	if result.Total != 1 || len(result.Flows) != 1 {
		t.Fatalf("filtered result = total %d len %d, want 1", result.Total, len(result.Flows))
	}
	flow := result.Flows[0]
	if flow.AppProtocol != "TLS" || flow.SNI != "api.example.com" || flow.TLSALPN != "h2, http/1.1" || flow.DstDomain != "api.example.com" {
		t.Fatalf("flow TLS metadata = app=%q sni=%q alpn=%q domain=%q", flow.AppProtocol, flow.SNI, flow.TLSALPN, flow.DstDomain)
	}
}

func TestFlowRiskReasonsForSuspiciousEndpointAndVolume(t *testing.T) {
	agg := newFlowAggregator()
	agg.RecordConnectionContext("10.0.0.2", "203.0.113.10", 40000, 4444, "TCP", "nc", 10, "outgoing", "ESTABLISHED", &pb.Event{Pid: 10, Comm: "nc", NetBytes: 12 * 1024 * 1024})
	result := agg.Query(networkFlowQuery{ShowHistoric: true, Limit: 10})
	if result.Total != 1 || len(result.Flows) != 1 {
		t.Fatalf("result total=%d len=%d, want 1", result.Total, len(result.Flows))
	}
	flow := result.Flows[0]
	if flow.RiskScore < 0.80 || flow.RiskLevel != "high" {
		t.Fatalf("risk = %.2f/%s, want high >=0.80", flow.RiskScore, flow.RiskLevel)
	}
	joined := strings.Join(flow.RiskReasons, "; ")
	for _, want := range []string{"suspicious IP scope", "suspicious endpoint pattern", "large outbound volume"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("RiskReasons = %#v, missing %q", flow.RiskReasons, want)
		}
	}
}
