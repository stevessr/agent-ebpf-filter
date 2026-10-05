package tls

import (
	"testing"
)

func TestDefaultTLSCaptureRuleAllowsAllWhenEmpty(t *testing.T) {
	rules := NewTLSCaptureRuleStore()

	// With no rules, all events are allowed
	if !rules.Allows(TLSPlaintextEvent{PID: 4242, TGID: 4242, Comm: "claude"}) {
		t.Fatal("empty rules should allow all events (claude)")
	}
	if !rules.Allows(TLSPlaintextEvent{PID: 9999, TGID: 9999, Comm: "curl"}) {
		t.Fatal("empty rules should allow all events (curl)")
	}
}

func TestCustomTLSCaptureRuleMatchesCommonFields(t *testing.T) {
	rules := NewTLSCaptureRuleStore()
	rules.Replace([]TLSCaptureRule{
		{ID: "custom", Name: "Custom", Enabled: true, Scope: "custom", Comms: []string{"node"}, Hosts: []string{"api.example.com"}, Methods: []string{"POST"}, Libraries: []string{"OpenSSL"}, Directions: []string{"send"}},
	})

	allowed := TLSPlaintextEvent{Comm: "node", Host: "api.example.com", Method: "POST", Lib: "openssl", Direction: "send"}
	if !rules.Allows(allowed) {
		t.Fatal("custom rule should match normalized fields")
	}
	blocked := allowed
	blocked.Host = "other.example.com"
	if rules.Allows(blocked) {
		t.Fatal("custom rule should reject non-matching host")
	}
}


func TestTLSExecutablePathScopeIsFailClosed(t *testing.T) {
	rules := NewTLSCaptureRuleStore()
	if rules.AllowsExecutablePath("/usr/bin/curl") {
		t.Fatal("empty executable path rules must deny auto-attach")
	}

	rules.Replace([]TLSCaptureRule{
		{
			ID:      "scoped",
			Name:    "Scoped TLS",
			Enabled: true,
			Scope:   "custom",
			Paths:   []string{"/usr/bin/codex", "/opt/agents/**"},
		},
	})
	for _, path := range []string{"/usr/bin/codex", "/opt/agents/claude", "/opt/agents/nested/codex"} {
		if !rules.AllowsExecutablePath(path) {
			t.Fatalf("expected executable path %q to be allowed", path)
		}
	}
	for _, path := range []string{"/usr/bin/curl", "/opt/agent-other/codex"} {
		if rules.AllowsExecutablePath(path) {
			t.Fatalf("expected executable path %q to be denied", path)
		}
	}
}

func TestTLSExecutablePathScopeRejectsRootWidePrefix(t *testing.T) {
	rules := NewTLSCaptureRuleStore()
	rules.Replace([]TLSCaptureRule{{
		ID:      "root",
		Name:    "Root",
		Enabled: true,
		Scope:   "custom",
		Paths:   []string{"/**"},
	}})
	if rules.AllowsExecutablePath("/usr/bin/curl") {
		t.Fatal("/** must not enable host-wide TLS auto-attach")
	}
}

func TestTLSExecutablePathScopeIgnoresDisabledRules(t *testing.T) {
	rules := NewTLSCaptureRuleStore()
	rules.Replace([]TLSCaptureRule{{
		ID:      "disabled",
		Name:    "Disabled",
		Enabled: false,
		Scope:   "custom",
		Paths:   []string{"/usr/bin/curl"},
	}})
	if rules.AllowsExecutablePath("/usr/bin/curl") {
		t.Fatal("disabled path rule must not permit auto-attach")
	}
	if got := rules.ExecutablePaths(); len(got) != 0 {
		t.Fatalf("ExecutablePaths() = %v, want empty", got)
	}
}
