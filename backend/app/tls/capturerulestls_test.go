package tls

import (
	"testing"
)

// ---- moved from backend/zz_merged_backend_test.go section capturerulestls_test.go ----

func TestDefaultTLSCaptureRuleAllowsAgentContextOnly(t *testing.T) {
	rules := NewTLSCaptureRuleStore()
	pid := uint32(4242)
	trackedProcessContexts.Set(pid, processContext{AgentRunID: "run-ssl"})
	t.Cleanup(func() { trackedProcessContexts.Delete(pid) })

	if !rules.Allows(TLSPlaintextEvent{PID: pid, TGID: pid, Comm: "claude"}) {
		t.Fatal("default rule should allow agent CLI tagged process")
	}
	if rules.Allows(TLSPlaintextEvent{PID: 9999, TGID: 9999, Comm: "curl"}) {
		t.Fatal("default rule should reject untagged process")
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
