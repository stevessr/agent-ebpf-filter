package tls

import (
	"testing"
)

// ---- moved from backend/zz_merged_backend_test.go section capturerulestls_test.go ----

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
