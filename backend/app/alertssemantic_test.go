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

func TestSemanticAlertsDetectToolBaselineDriftBeforeRecording(t *testing.T) {
	previousBaseline := toolBaseline
	toolBaseline = newToolBaselineStore()
	t.Cleanup(func() { toolBaseline = previousBaseline })
	resetSemanticAlertState()

	for index, behavior := range []struct {
		comm      string
		eventType string
	}{
		{comm: "git", eventType: "baseline_exec"},
		{comm: "rg", eventType: "baseline_read"},
		{comm: "cat", eventType: "baseline_open"},
	} {
		event := &pb.Event{
			Pid:      uint32(900 + index),
			ToolName: "baseline-review-tool",
			Comm:     behavior.comm,
			Type:     behavior.eventType,
			Path:     "/workspace",
		}
		enrichEventContext(event)
		if alert := findSemanticAlertCode(buildSemanticAlerts(event), "TOOL_BEHAVIOR_DRIFT"); alert != nil {
			t.Fatalf("baseline warm-up emitted drift: %+v", alert)
		}
	}
	for observation := 3; observation < toolBaselineMinObservations; observation++ {
		event := &pb.Event{
			Pid:      uint32(1000 + observation),
			ToolName: "baseline-review-tool",
			Comm:     "git",
			Type:     "baseline_exec",
			Path:     "/workspace",
		}
		enrichEventContext(event)
		if alert := findSemanticAlertCode(buildSemanticAlerts(event), "TOOL_BEHAVIOR_DRIFT"); alert != nil {
			t.Fatalf("known baseline warm-up emitted drift: %+v", alert)
		}
	}

	driftEvent := &pb.Event{
		Pid:      999,
		ToolName: "baseline-review-tool",
		Comm:     "curl",
		Type:     "baseline_network",
		Path:     "/usr/bin/curl",
	}
	enrichEventContext(driftEvent)
	alert := findSemanticAlertCode(buildSemanticAlerts(driftEvent), "TOOL_BEHAVIOR_DRIFT")
	if alert == nil || !strings.Contains(alert.GetExtraInfo(), "baseline drift") {
		t.Fatalf("expected detect-before-record drift alert, got %+v", alert)
	}
	if repeated := findSemanticAlertCode(buildSemanticAlerts(driftEvent), "TOOL_BEHAVIOR_DRIFT"); repeated != nil {
		t.Fatalf("recorded behavior emitted drift twice: %+v", repeated)
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
