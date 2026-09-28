package detectionengineering

import "testing"

func sampleRule() Rule {
	return Rule{
		ID: "multi-signal-egress", Scope: "trace", WindowMS: 1000, MinSignals: 2,
		Signals: []Signal{
			{Field: "eventType", Value: "openat"},
			{Field: "comm", Value: "curl"},
		},
	}
}

func TestLintRejectsBrittleOrSingleDimensionRules(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Rule)
	}{
		{"path", func(r *Rule) { r.Signals[1].Value = "/tmp/only-this-host" }},
		{"ip", func(r *Rule) { r.Signals[1].Value = "203.0.113.9" }},
		{"host", func(r *Rule) { r.Signals[1].Value = "api.github.com" }},
		{"unknown field", func(r *Rule) { r.Signals[1].Field = "secretPayload" }},
		{"single dimension", func(r *Rule) { r.Signals[1].Field = "eventType" }},
		{"duplicate", func(r *Rule) { r.Signals[1] = r.Signals[0] }},
		{"invalid window", func(r *Rule) { r.WindowMS = 600001 }},
		{"invalid threshold", func(r *Rule) { r.MinSignals = 1 }},
		{"invalid scope", func(r *Rule) { r.Scope = "global" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := sampleRule()
			tc.edit(&r)
			if len(Lint(r)) == 0 {
				t.Fatalf("expected lint failure for %#v", r)
			}
		})
	}
	if issues := Lint(sampleRule()); len(issues) != 0 {
		t.Fatalf("valid rule rejected: %#v", issues)
	}
}

func TestReplayHeldoutAndBenignReviewGate(t *testing.T) {
	events := []Event{
		{ID: "a1", Timestamp: 1000, TraceID: "author", EventType: "openat", Comm: "bash"},
		{ID: "a2", Timestamp: 1100, TraceID: "author", EventType: "connect", Comm: "curl"},
		{ID: "h1", Timestamp: 2000, TraceID: "heldout", EventType: "openat", Comm: "bash"},
		{ID: "h2", Timestamp: 2100, TraceID: "heldout", EventType: "connect", Comm: "curl"},
		{ID: "b1", Timestamp: 2200, TraceID: "normal", EventType: "openat", Comm: "git"},
		{ID: "orphan", Timestamp: 2200, EventType: "openat", Comm: "curl"},
	}
	labels := map[string]string{
		"trace:author": "authoring_attack", "trace:heldout": "holdout_attack",
		"trace:normal": "benign",
	}
	report := Replay(sampleRule(), events, labels)
	if !report.Valid || !report.EligibleForHumanReview || report.EnforcementApplied {
		t.Fatalf("unexpected gates: %#v", report)
	}
	if report.SkippedUnscoped != 1 || report.DetectedScopes != 2 || len(report.Findings) != 2 {
		t.Fatalf("unexpected matching: %#v", report)
	}
	if report.Metrics.HoldoutRecall == nil || *report.Metrics.HoldoutRecall != 1 ||
		report.Metrics.FalsePositiveRate == nil || *report.Metrics.FalsePositiveRate != 0 {
		t.Fatalf("unexpected metrics: %#v", report.Metrics)
	}
	if len(report.Findings[0].EvidenceEventIDs) != 2 {
		t.Fatalf("missing bounded evidence: %#v", report.Findings[0])
	}
}

func TestReplayScopeAndWindowIsolation(t *testing.T) {
	rule := sampleRule()
	events := []Event{
		{ID: "x1", Timestamp: 1000, TraceID: "x", EventType: "openat"},
		{ID: "y1", Timestamp: 1100, TraceID: "y", Comm: "curl"}, // different trace
		{ID: "x2", Timestamp: 2101, TraceID: "x", Comm: "curl"}, // too late
	}
	result := Replay(rule, events, nil)
	if len(result.Findings) != 0 || result.DetectedScopes != 0 || result.EligibleForHumanReview {
		t.Fatalf("cross-scope or out-of-window correlation: %#v", result)
	}
	rule.Scope = "pid"
	events = []Event{
		{ID: "p1", Timestamp: 1000, PID: 41, EventType: "openat"},
		{ID: "p2", Timestamp: 1001, PID: 42, Comm: "curl"},
		{ID: "p3", Timestamp: 1002, PID: 41, Comm: "curl"},
	}
	result = Replay(rule, events, nil)
	if result.DetectedScopes != 1 || result.Findings[0].Scope != "pid:41" {
		t.Fatalf("unexpected PID correlation: %#v", result)
	}
}

func TestReplayBenignFalsePositiveAndUnverifiedLabels(t *testing.T) {
	events := []Event{
		{ID: "b1", Timestamp: 1000, TraceID: "benign", EventType: "openat", Comm: "curl"},
		{ID: "a1", Timestamp: 1100, TraceID: "attack", EventType: "openat", Comm: "curl"},
	}
	report := Replay(sampleRule(), events, map[string]string{
		"trace:benign": "benign", "trace:attack": "holdout_attack",
	})
	if report.EligibleForHumanReview || report.Metrics.BenignFlagged != 1 {
		t.Fatalf("false-positive should fail review gate: %#v", report)
	}
	report = Replay(sampleRule(), events, map[string]string{"trace:missing": "holdout_attack"})
	if report.Valid || len(report.Findings) != 0 {
		t.Fatalf("missing evidence for label must fail validation: %#v", report)
	}
	report = Replay(sampleRule(), events, map[string]string{"trace:attack": "BLOCK"})
	if report.Valid {
		t.Fatal("policy decision must not be treated as ground truth")
	}
	report = Replay(sampleRule(), events, nil)
	if report.Metrics.HoldoutRecall != nil || report.Metrics.FalsePositiveRate != nil ||
		report.EligibleForHumanReview || report.UnlabeledScopes != 2 {
		t.Fatalf("unlabeled traffic must not yield measured accuracy: %#v", report)
	}
}

func TestReplayOutOfOrderAndEpisodeDedup(t *testing.T) {
	rule := sampleRule()
	events := []Event{
		{ID: "late", Timestamp: 1500, TraceID: "run", EventType: "openat", Comm: "curl"},
		{ID: "early", Timestamp: 1000, TraceID: "run", EventType: "openat", Comm: "curl"},
		{ID: "repeat", Timestamp: 1501, TraceID: "run", EventType: "openat", Comm: "curl"},
		{ID: "expired", Timestamp: 3000, TraceID: "run", EventType: "openat", Comm: "curl"},
	}
	report := Replay(rule, events, nil)
	if !report.Valid || len(report.Findings) != 2 || report.DetectedScopes != 1 {
		t.Fatalf("expected one episode before timeout and a new episode after it: %#v", report)
	}
	if report.Findings[0].FirstTimestamp != 1000 || report.Findings[0].LastTimestamp != 1000 {
		t.Fatalf("replay must use chronological telemetry: %#v", report.Findings[0])
	}
}
