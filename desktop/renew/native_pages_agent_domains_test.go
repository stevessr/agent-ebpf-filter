package main

import (
	"encoding/json"
	"testing"
)

func TestNormalizedDestinationHostSafety(t *testing.T) {
	for _, tc := range []struct{ input, host, kind string }{
		{"api.openai.com:443", "api.openai.com", "域名"},
		{"HTTPS://API.OPENAI.COM/v1", "", ""}, // scheme must be explicit lowercase
		{"https://API.OpenAI.com:443/v1", "api.openai.com", "域名"},
		{"1.2.3.4:443", "1.2.3.4", "IP"},
		{"[2001:db8::1]:443", "2001:db8::1", "IP"},
		{"2001:db8::1", "2001:db8::1", "IP"},
		{"[DOMAIN:16]", "", ""},
		{"/tmp/private.txt", "", ""},
		{"example.com.evil.test", "example.com.evil.test", "域名"},
		{"http://user:pass@api.openai.com/", "", ""},
		{"http://foo.example@api.openai.com/", "", ""},
		{"localhost", "", ""},
		{"exa_mple.com", "", ""},
		{"api.openai.com:bad", "", ""},
	} {
		host, kind := normalizedDestinationHost(tc.input)
		if host != tc.host || kind != tc.kind {
			t.Errorf("normalizedDestinationHost(%q) = %q/%q, want %q/%q", tc.input, host, kind, tc.host, tc.kind)
		}
	}
}

func testAgentDomainEvents() []eventSummary {
	return []eventSummary{
		{EventID: "root", PID: 100, RootAgentPID: 100, Comm: "codex", Tag: "AI Agent", HasAgentContext: true, AgentRunID: "r1", ReceivedAtMS: 100, Type: "execve"},
		{EventID: "domain", PID: 102, RootAgentPID: 100, Comm: "bash", Tag: "AI Agent", HasAgentContext: true, AgentRunID: "r1", Network: true, Type: "connect", Domain: "api.openai.com", Target: "203.0.113.10:443", RiskScore: 65, ReceivedAtMS: 110},
		{EventID: "domain2", PID: 102, RootAgentPID: 100, Comm: "bash", Tag: "AI Agent", HasAgentContext: true, AgentRunID: "r1", Network: true, Type: "connect", Target: "api.openai.com:443", ReceivedAtMS: 111},
		{EventID: "different", PID: 102, RootAgentPID: 100, Comm: "bash", Tag: "AI Agent", HasAgentContext: true, AgentRunID: "r1", Network: true, Target: "api.openai.com.evil.test:443", ReceivedAtMS: 112},
		{EventID: "only-ip", PID: 102, RootAgentPID: 100, Comm: "bash", Tag: "AI Agent", HasAgentContext: true, AgentRunID: "r1", Network: true, Target: "192.0.2.9:443", ReceivedAtMS: 113},
		{EventID: "not-agent", PID: 800, Comm: "curl", Network: true, Target: "api.openai.com:443", ReceivedAtMS: 114},
		{EventID: "not-network", PID: 102, RootAgentPID: 100, Comm: "bash", HasAgentContext: true, AgentRunID: "r1", Type: "openat", Target: "api.openai.com", ReceivedAtMS: 115},
	}
}

func TestAggregateAgentDomainsAndExactDrilldown(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = testAgentDomainEvents()
	a.page = "域名"
	rows := a.agentDomainRows()
	if len(rows) != 3 {
		t.Fatalf("expected 3 Agent domain / IP groups; got %+v", rows)
	}
	var target agentDomainRow
	for _, row := range rows {
		if row.Target == "api.openai.com" {
			target = row
			break
		}
	}
	if target.Agent != "Codex" || target.Events != 2 || target.Alerts != 1 || target.Risk != "需关注" || target.Sessions != 1 {
		t.Fatalf("root Agent ownership/domain aggregate incorrect: %+v", target)
	}
	a.domainShowIPs = false
	if got := a.matchingAgentDomainRows(); len(got) != 2 {
		t.Fatalf("expected IP-only group to be hidden, got %d", len(got))
	}
	a.domainRiskOnly = true
	if got := a.matchingAgentDomainRows(); len(got) != 1 || got[0].Target != "api.openai.com" {
		t.Fatalf("risk filtering incorrect: %+v", got)
	}
	a.openAgentDomainEvents(target)
	if a.page != "事件" || a.eventReturnPage != "域名" {
		t.Fatalf("drilldown should preserve return route, got %q / %q", a.page, a.eventReturnPage)
	}
	filtered := a.filteredEvents()
	if len(filtered) != 2 || filtered[0].EventID != "domain" || filtered[1].EventID != "domain2" {
		t.Fatalf("exact Agent-domain filter leaked unrelated events: %+v", filtered)
	}
	a.clearEventFilters()
	if a.eventDomainTarget != "" || a.eventDomainAgent != "" || a.eventDomainKind != "" || len(a.filteredEvents()) != len(a.events) {
		t.Fatal("clearing filters must remove domain drilldown constraints")
	}
}

func TestCompactDomainFieldsStaySeparate(t *testing.T) {
	var e eventSummary
	err := json.Unmarshal([]byte(`{"eventId":"d1","type":"connect","network":true,"domain":"api.example.com","netEndpoint":"192.0.2.1:443","target":"api.example.com"}`), &e)
	if err != nil {
		t.Fatal(err)
	}
	if e.Domain != "api.example.com" || e.NetEndpoint != "192.0.2.1:443" {
		t.Fatalf("structured evidence lost: %+v", e)
	}
	host, kind := agentEventDestination(e)
	if host != "api.example.com" || kind != "域名" {
		t.Fatalf("structured domain should precede endpoint: %q/%q", host, kind)
	}
}
