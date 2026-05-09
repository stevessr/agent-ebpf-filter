package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-ebpf-filter/pb"
	"github.com/gin-gonic/gin"
)

func TestHandleNetworkFlowsSupportsFilterAndFlowDetail(t *testing.T) {
	orig := networkFlowAggregator
	networkFlowAggregator = newFlowAggregator()
	defer func() { networkFlowAggregator = orig }()

	networkFlowAggregator.RecordConnectionContext("10.0.0.2", "93.184.216.34", 42000, 443, "TCP", "curl", 123, "outgoing", "ESTABLISHED", &pb.Event{
		Pid:        123,
		Comm:       "curl",
		NetBytes:   512,
		AgentRunId: "run-flow-test",
		ToolCallId: "tool-flow-test",
	})
	networkFlowAggregator.ApplyProtocolMetadata("10.0.0.2", "93.184.216.34", 42000, 443, "TCP", &protoDetectionEntry{
		AppProtocol: AppProtoTLS,
		SNI:         "api.example.com",
		ALPN:        "h2",
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/network/flows", handleNetworkFlows)
	router.GET("/network/flows/:flowID", handleNetworkFlowByID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/network/flows?filter=process:curl+sni:api.example.com&showHistoric=true&limit=10", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("flows status = %d body=%s", rec.Code, rec.Body.String())
	}
	var list networkFlowQueryResult
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode flows response: %v", err)
	}
	if list.Total != 1 || len(list.Flows) != 1 {
		t.Fatalf("flows total=%d len=%d, want 1", list.Total, len(list.Flows))
	}
	flow := list.Flows[0]
	if flow.SNI != "api.example.com" || flow.TLSALPN != "h2" || len(flow.AgentRunIDs) != 1 || flow.AgentRunIDs[0] != "run-flow-test" {
		t.Fatalf("flow metadata = sni=%q alpn=%q agents=%#v", flow.SNI, flow.TLSALPN, flow.AgentRunIDs)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/network/flows/"+flow.FlowID, nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("flow detail status = %d body=%s", rec.Code, rec.Body.String())
	}
	var detail NetworkFlowSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detail.FlowID != flow.FlowID || detail.SNI != "api.example.com" {
		t.Fatalf("detail = id %q sni %q, want %q/api.example.com", detail.FlowID, detail.SNI, flow.FlowID)
	}
}

func TestHandleNetworkFlowJSONLExportIncludesAttributionAndDPI(t *testing.T) {
	orig := networkFlowAggregator
	networkFlowAggregator = newFlowAggregator()
	defer func() { networkFlowAggregator = orig }()

	networkFlowAggregator.RecordConnectionContext("10.0.0.2", "93.184.216.34", 42000, 80, "TCP", "curl", 123, "outgoing", "ESTABLISHED", &pb.Event{
		Pid:        123,
		Comm:       "curl",
		NetBytes:   256,
		AgentRunId: "run-jsonl",
		ToolCallId: "tool-jsonl",
	})
	networkFlowAggregator.ApplyProtocolMetadata("10.0.0.2", "93.184.216.34", 42000, 80, "TCP", &protoDetectionEntry{
		AppProtocol: AppProtoHTTP,
		HTTPHost:    "example.com",
		HTTPMethod:  "GET",
	})

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/network/export/jsonl?filter=host:example.com&showHistoric=true", nil)
	handleNetworkFlowJSONLExport(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("jsonl status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/x-ndjson") {
		t.Fatalf("Content-Type = %q, want application/x-ndjson", got)
	}
	scanner := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	if !scanner.Scan() {
		t.Fatalf("expected one JSONL row, body=%q", rec.Body.String())
	}
	var row NetworkFlowSummary
	if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
		t.Fatalf("decode jsonl row: %v", err)
	}
	if row.HTTPHost != "example.com" || row.HTTPMethod != "GET" || len(row.AgentRunIDs) != 1 || row.AgentRunIDs[0] != "run-jsonl" {
		t.Fatalf("jsonl row metadata = host=%q method=%q agents=%#v", row.HTTPHost, row.HTTPMethod, row.AgentRunIDs)
	}
	if scanner.Scan() {
		t.Fatalf("expected one JSONL row, got extra %q", scanner.Text())
	}
}

func TestHandleDNSCacheReturnsEntries(t *testing.T) {
	orig := dnsCorrelation
	dnsCorrelation = newDNSCache()
	defer func() { dnsCorrelation = orig }()
	dnsCorrelation.Record("example.com", "93.184.216.34")

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/network/dns-cache", nil)
	handleDNSCache(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("dns cache status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "example.com") || !strings.Contains(rec.Body.String(), "93.184.216.34") {
		t.Fatalf("dns cache body missing entry: %s", rec.Body.String())
	}
}
