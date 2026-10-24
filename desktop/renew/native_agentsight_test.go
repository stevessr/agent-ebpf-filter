package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentSightUsageAPIFetchIsReadOnlyAndAuthenticated(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet {
			t.Errorf("unexpected mutation method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/agentsight/report/usage" {
			t.Errorf("unexpected API path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "2000" {
			t.Errorf("missing bounded limit: %s", r.URL.RawQuery)
		}
		if got := r.Header.Get("X-API-KEY"); got != "test-token" {
			t.Errorf("access token was not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(agentSightUsageResponse{
			Limit: 2000, Source: "runtime",
			Summary: agentSightUsageSummary{
				EventCount: 3, UsageEvents: 1, InputTokens: 200, OutputTokens: 25,
				Coverage: "explicit_usage_only",
				Sessions: []agentSightUsageSession{{ID: "run-1", EventCount: 2, InputTokens: 200}},
				Models: []agentSightUsageModel{{Model: "example", InputTokens: 200}},
			},
		})
	}))
	defer server.Close()

	client := newAPIClient(server.URL, "test-token")
	report, err := client.agentSightUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || report.Summary.EventCount != 3 || report.Summary.InputTokens != 200 {
		t.Fatalf("incorrect AgentSight response: %+v", report)
	}
	if len(report.Summary.Sessions) != 1 || report.Summary.Sessions[0].ID != "run-1" {
		t.Fatalf("session metadata missing: %+v", report.Summary.Sessions)
	}
}

func TestAgentSightUsageUnknownAndSessionNavigation(t *testing.T) {
	if got := agentSightUsageCoverageText(agentSightUsageSummary{EventCount: 8}); !strings.Contains(got, "未知") {
		t.Fatalf("missing token coverage must not be treated as zero: %s", got)
	}
	if got := agentSightUsageCoverageText(agentSightUsageSummary{
		EventCount: 8, UsageEvents: 2, UnattributedEvents: 4,
	}); !strings.Contains(got, "2") || !strings.Contains(got, "4") {
		t.Fatalf("coverage counts missing: %s", got)
	}
	if key, ok := agentSightEventSessionKey("run-A"); !ok || key != "run:run-A" {
		t.Fatalf("expected native run key, got %q %v", key, ok)
	}
	if key, ok := agentSightEventSessionKey("trace:123"); ok || key != "" {
		t.Fatalf("trace id should not be projected onto a native run, got %q", key)
	}
	if label := agentSightSessionLabel(strings.Repeat("a", 120)); len(label) >= 120 {
		t.Fatalf("session label must be truncated for native layout")
	}
}

func TestAgentSightEffectSummaryUsesOnlyAgentEvidence(t *testing.T) {
	rows := []eventSummary{
		{AgentRunID: "run-1", Type: "openat", EventType: 4, Comm: "codex"},
		{AgentRunID: "run-1", Type: "network", Network: true, Decision: "BLOCK", Comm: "codex", RiskScore: 90},
		{AgentRunID: "run-1", Type: "semantic_alert", Comm: "codex"},
		{Tag: "Shell", Comm: "bash", Type: "network", Network: true},
	}
	result := buildAgentSightEffects(rows)
	if result.AgentEvents != 3 || result.NetworkEvents != 1 || result.DeniedEvents != 1 {
		t.Fatalf("unattributed shell included or policy decisions lost: %+v", result)
	}
	if result.RiskEvents < 2 {
		t.Fatalf("risk events missing: %+v", result)
	}
}

func TestAgentSightUnicodeSessionLabel(t *testing.T) {
	id := strings.Repeat("会", 110)
	label := agentSightSessionLabel(id)
	if !strings.Contains(label, "…") || !strings.HasPrefix(label, "会") {
		t.Fatalf("Unicode label was not truncated safely: %q", label)
	}
}
