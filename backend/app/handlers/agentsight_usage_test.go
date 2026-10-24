package handlers

import "testing"

func TestAgentSightUsageSummaryGroupsAndDeduplicates(t *testing.T) {
	events := []AgentSightExportEvent{
		{
			ID: "one", Timestamp: 100, Source: "hook", Comm: "claude",
			SpanID: "span-1", Data: map[string]any{
				"agent_run_id": "run-A", "model": "claude-test",
				"usage": map[string]any{
					"input_tokens": float64(100), "output_tokens": float64(30),
					"cache_read_input_tokens": float64(40),
				},
			},
		},
		{
			ID: "duplicate", Timestamp: 101, SpanID: "span-1",
			Data: map[string]any{
				"agent_run_id": "run-A", "model": "claude-test",
				"usage": map[string]any{"input_tokens": float64(100), "output_tokens": float64(30)},
			},
		},
		{
			ID: "two", Timestamp: 102, Data: map[string]any{
				"agent_run_id": "run-B",
				"token_usage": map[string]any{
					"prompt_tokens": "20", "completion_tokens": float64(8),
				},
			},
		},
		{
			ID: "no-usage", Timestamp: 103, Data: map[string]any{
				"agent_run_id": "run-A", "prompt_len": 700,
			},
		},
		{
			ID: "unattributed", Timestamp: 104, Data: map[string]any{
				"input_tokens": float64(12), "output_tokens": float64(3),
			},
		},
	}
	report := BuildAgentSightUsageSummary(events, "")
	if report.EventCount != 5 || report.UnattributedEvents != 1 || report.UsageEvents != 3 {
		t.Fatalf("incorrect coverage: %+v", report)
	}
	if report.InputTokens != 132 || report.OutputTokens != 41 || report.CacheReadTokens != 40 {
		t.Fatalf("incorrect tokens: %+v", report)
	}
	if len(report.Sessions) != 2 || report.Sessions[0].ID != "run-A" || report.Sessions[0].EventCount != 3 {
		t.Fatalf("incorrect sessions: %+v", report.Sessions)
	}
	if len(report.Models) != 2 || report.Models[0].Model != "claude-test" {
		t.Fatalf("incorrect models: %+v", report.Models)
	}
	filtered := BuildAgentSightUsageSummary(events, "run-B")
	if filtered.EventCount != 1 || filtered.InputTokens != 20 || len(filtered.Sessions) != 1 {
		t.Fatalf("session filter leaked another run: %+v", filtered)
	}
}

func TestAgentSightUsageSummaryUnknownIsNotZero(t *testing.T) {
	report := BuildAgentSightUsageSummary([]AgentSightExportEvent{{
		ID: "no-tokens", TraceID: "trace-A", Data: map[string]any{
			"prompt_len": 500, "latency_ms": 2000,
		},
	}}, "")
	if report.UsageEvents != 0 || report.Coverage != "explicit_usage_only" {
		t.Fatalf("uncaptured usage should remain unknown: %+v", report)
	}
	if len(report.Sessions) != 1 || report.Sessions[0].ID != "trace:trace-A" {
		t.Fatalf("trace fallback missing: %+v", report.Sessions)
	}
	zero := BuildAgentSightUsageSummary([]AgentSightExportEvent{{
		Data: map[string]any{"usage": map[string]any{"input_tokens": float64(0)}},
	}}, "")
	if zero.UsageEvents != 1 || zero.InputTokens != 0 {
		t.Fatalf("explicitly captured zero must be distinguishable: %+v", zero)
	}
}

func TestAgentSightUsageSummaryRejectsMalformedCounters(t *testing.T) {
	report := BuildAgentSightUsageSummary([]AgentSightExportEvent{{
		Data: map[string]any{"usage": map[string]any{
			"input_tokens": -10, "output_tokens": "not a number",
		}},
	}}, "")
	if report.UsageEvents != 0 || report.InputTokens != 0 {
		t.Fatalf("malformed usage counted: %+v", report)
	}
	report = BuildAgentSightUsageSummary([]AgentSightExportEvent{{
		Data: map[string]any{
			"envelope": map[string]any{"agent_run_id": "run-C", "llm_call": map[string]any{
				"model": "gpt-test", "input_tokens": float64(11), "output_tokens": float64(2),
			}},
		},
	}}, "")
	if report.InputTokens != 11 || len(report.Sessions) != 1 || report.Sessions[0].ID != "run-C" {
		t.Fatalf("nested envelope ignored: %+v", report)
	}
}
