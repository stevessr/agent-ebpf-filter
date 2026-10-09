package app

import (
	"agent-ebpf-filter/pb"
	"strings"
	"testing"
)

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

func TestSemanticFileContentionIgnoresUnresolvedTargets(t *testing.T) {
	for _, target := range []string{"", "write", "file write", "file_write", "socket 5", "pipe:[123]", "fd:3", "relative.txt"} {
		t.Run(target, func(t *testing.T) {
			resetSemanticAlertState()
			for _, run := range []string{"run-one", "run-two"} {
				event := &pb.Event{
					Pid: 200, Tgid: 200, Type: "write", EventType: pb.EventType_WRITE,
					AgentRunId: run, Comm: "fish", Path: target,
				}
				if alert := findSemanticAlertCode(buildSemanticAlerts(event), "MULTI_AGENT_FILE_CONTENTION"); alert != nil {
					t.Fatalf("unresolved path %q must not trigger contention: %+v", target, alert)
				}
			}
		})
	}
	// Even with an absolute cwd, an adapter's synthetic "file write" label
	// must not turn into a plausible /workspace/file write pathname.
	resetSemanticAlertState()
	for _, run := range []string{"run-one", "run-two"} {
		event := &pb.Event{
			Pid: 200, Type: "write", EventType: pb.EventType_WRITE,
			AgentRunId: run, Cwd: "/workspace", Path: "file write",
		}
		if alert := findSemanticAlertCode(buildSemanticAlerts(event), "MULTI_AGENT_FILE_CONTENTION"); alert != nil {
			t.Fatalf("synthetic file label became an alert target: %+v", alert)
		}
	}
}

func TestSemanticFileContentionRequiresDistinctAgentEvidence(t *testing.T) {
	resetSemanticAlertState()
	for _, pid := range []uint32{201, 301} {
		event := &pb.Event{
			Pid: pid, Tgid: pid, Type: "write", EventType: pb.EventType_WRITE,
			Path: "/workspace/shared.txt", Comm: "fish",
		}
		if alert := findSemanticAlertCode(buildSemanticAlerts(event), "MULTI_AGENT_FILE_CONTENTION"); alert != nil {
			t.Fatalf("different bare PIDs are not proof of different agents: %+v", alert)
		}
	}
	// Different tool calls/runs in a single known Agent process are not
	// separate agents; the stable root PID takes precedence.
	resetSemanticAlertState()
	for _, run := range []string{"run-one", "run-two"} {
		event := &pb.Event{
			Pid: 201, RootAgentPid: 100, AgentRunId: run,
			Type: "write", EventType: pb.EventType_WRITE,
			Path: "/workspace/shared.txt",
		}
		if alert := findSemanticAlertCode(buildSemanticAlerts(event), "MULTI_AGENT_FILE_CONTENTION"); alert != nil {
			t.Fatalf("one agent root emitted a false cross-agent alert: %+v", alert)
		}
	}
}

func TestSemanticFileContentionResolvesRealRelativePaths(t *testing.T) {
	resetSemanticAlertState()
	first := &pb.Event{
		Pid: 201, RootAgentPid: 101, Type: "write", EventType: pb.EventType_WRITE,
		Path: "shared.txt", Cwd: "/workspace",
	}
	second := &pb.Event{
		Pid: 301, RootAgentPid: 102, Type: "write", EventType: pb.EventType_WRITE,
		Path: "file write", ExtraPath: "/workspace/shared.txt", Cwd: "/other",
	}
	if alerts := buildSemanticAlerts(first); hasSemanticAlertCode(alerts, "MULTI_AGENT_FILE_CONTENTION") {
		t.Fatalf("first tracked write should not alert: %+v", alerts)
	}
	alert := findSemanticAlertCode(buildSemanticAlerts(second), "MULTI_AGENT_FILE_CONTENTION")
	if alert == nil || alert.GetPath() != "/workspace/shared.txt" {
		t.Fatalf("expected a real, resolved path for distinct agent roots; got %+v", alert)
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
