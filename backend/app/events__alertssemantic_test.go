package app

import (
	"agent-ebpf-filter/pb"
	"strings"
	"testing"
)

// ---- moved from backend/zz_merged_backend_test.go section alertssemantic_test.go ----

func TestSemanticAlertsDetectAgenticResourceLoopFromSafeMetadata(t *testing.T) {
	resetSemanticAlertState()

	base := &pb.Event{
		Pid:          100,
		Tgid:         100,
		RootAgentPid: 100,
		AgentRunId:   "run-loop",
		ToolName:     "chat",
		Comm:         "agent",
	}
	promptDigest := digestHookText("repeat this prompt")

	for i := 0; i < semanticPromptLoopThreshold; i++ {
		event := cloneProtoEvent(base)
		event.Type = "native_hook"
		event.EventType = pb.EventType_NATIVE_HOOK
		event.ExtraInfo = "prompt_digest=" + promptDigest + " prompt_len=18"
		if alerts := buildSemanticAlerts(event); hasSemanticAlertCode(alerts, "RESOURCE_WASTING_LOOP") {
			t.Fatalf("prompt metadata alone should not produce resource loop alert")
		}
	}
	for i := 0; i < semanticAPILoopThreshold; i++ {
		event := cloneProtoEvent(base)
		event.Type = "network_connect"
		event.EventType = pb.EventType_NETWORK_CONNECT
		event.NetEndpoint = "api.openai.example:443"
		event.DstPort = 443
		_ = buildSemanticAlerts(event)
	}

	var finalAlerts []*pb.Event
	for i := 0; i < semanticFileIOLoopThreshold; i++ {
		event := cloneProtoEvent(base)
		event.Type = "openat"
		event.EventType = pb.EventType_OPENAT
		event.Path = "/tmp/agent-cache.json"
		finalAlerts = buildSemanticAlerts(event)
	}

	alert := findSemanticAlertCode(finalAlerts, "RESOURCE_WASTING_LOOP")
	if alert == nil {
		t.Fatalf("expected RESOURCE_WASTING_LOOP after repeated prompt metadata, API calls, and file I/O")
	}
	if !strings.Contains(alert.GetExtraInfo(), "repeated prompt metadata") {
		t.Fatalf("alert reason did not describe agentic loop correlation: %q", alert.GetExtraInfo())
	}
	if alert.GetTgid() != base.GetTgid() {
		t.Fatalf("alert tgid = %d, want %d", alert.GetTgid(), base.GetTgid())
	}
}

func TestSemanticAlertsDetectMultiAgentFileContention(t *testing.T) {
	resetSemanticAlertState()

	first := &pb.Event{
		Pid:        201,
		Tgid:       201,
		Type:       "write",
		EventType:  pb.EventType_WRITE,
		Path:       "/workspace/shared-plan.md",
		AgentRunId: "run-a",
		ToolName:   "editor",
		Comm:       "agent-a",
	}
	if alerts := buildSemanticAlerts(first); hasSemanticAlertCode(alerts, "MULTI_AGENT_FILE_CONTENTION") {
		t.Fatalf("first writer should not produce contention alert")
	}

	second := &pb.Event{
		Pid:        301,
		Tgid:       301,
		Type:       "write",
		EventType:  pb.EventType_WRITE,
		Path:       "/workspace/shared-plan.md",
		AgentRunId: "run-b",
		ToolName:   "editor",
		Comm:       "agent-b",
	}
	alert := findSemanticAlertCode(buildSemanticAlerts(second), "MULTI_AGENT_FILE_CONTENTION")
	if alert == nil {
		t.Fatalf("expected MULTI_AGENT_FILE_CONTENTION for different agent run touching same path")
	}
	if alert.GetPath() != "/workspace/shared-plan.md" {
		t.Fatalf("alert path = %q", alert.GetPath())
	}
	if !strings.Contains(alert.GetExtraInfo(), "run-a") || !strings.Contains(alert.GetExtraInfo(), "run-b") {
		t.Fatalf("alert reason should include both agent contexts: %q", alert.GetExtraInfo())
	}
}

func findSemanticAlertCode(alerts []*pb.Event, code string) *pb.Event {
	for _, alert := range alerts {
		if alert.GetType() == "semantic_alert" && alert.GetComm() == code {
			return alert
		}
	}
	return nil
}

func hasSemanticAlertCode(alerts []*pb.Event, code string) bool {
	return findSemanticAlertCode(alerts, code) != nil
}
