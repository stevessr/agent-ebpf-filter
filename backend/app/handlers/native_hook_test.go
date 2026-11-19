package handlers

import (
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestNativeHookProviderTagRecognizesHarnesses(t *testing.T) {
	tests := []struct {
		name, sourceCLI, userAgent, event, want string
	}{
		{name: "dsh header", sourceCLI: "dsh", want: "DeepSeek Harness"},
		{name: "Pi header", sourceCLI: "pi", want: "Pi"},
		{name: "OMP header", sourceCLI: "omp", want: "Oh My Pi"},
		{name: "Pi user agent", userAgent: "pi-coding-agent/1.0", want: "Pi"},
		{name: "OMP user agent", userAgent: "oh-my-pi/1.0", want: "Oh My Pi"},
		{name: "legacy Gemini event", event: "BeforeTool", want: "Gemini CLI"},
		{name: "unknown", want: "Native Hook"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nativeHookProviderTag(test.sourceCLI, test.userAgent, test.event); got != test.want {
				t.Fatalf("nativeHookProviderTag(%q, %q, %q) = %q, want %q", test.sourceCLI, test.userAgent, test.event, got, test.want)
			}
		})
	}
}

func TestEmitNativeHookEventDoesNotBlockWhenQueueFull(t *testing.T) {
	oldDeps := Deps
	t.Cleanup(func() { Deps = oldDeps })

	queue := make(chan *pb.Event, 1)
	queue <- &pb.Event{Type: "occupied"}
	Deps.BroadcastCh = queue
	Deps.SendTLSBridge = nil

	done := make(chan struct{})
	go func() {
		emitNativeHookEvent(&pb.Event{Type: "native_hook"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("native hook emitter blocked on a full broadcast queue")
	}

	if got := <-queue; got.GetType() != "occupied" {
		t.Fatalf("full queue contents changed: got %q", got.GetType())
	}
}

func TestClaudeSubAgentLifecycleProviderTag(t *testing.T) {
	for _, event := range []string{"SubagentStart", "SubagentStop"} {
		if got := nativeHookProviderTag("claude-subagent-start", "", event); got != "Claude Code" {
			t.Fatalf("hook source must retain provider identity: event=%s tag=%s", event, got)
		}
	}
}

func TestSubagentHookMetadataDigestDoesNotLeakIdentifier(t *testing.T) {
	payload := map[string]interface{}{
		"agent_id":   "private-agent-id-123",
		"agent_type": "Explore",
		"prompt":     "private user prompt",
	}
	extra := buildNativeHookExtraInfo(payload, "SubagentStart", "")
	if !strings.Contains(extra, "subagent_id_digest=sha256:") ||
		!strings.Contains(extra, "subagent_type=Explore") {
		t.Fatalf("missing privacy-safe SubAgent lifecycle metadata: %q", extra)
	}
	for _, secret := range []string{"private-agent-id-123", "private user prompt"} {
		if strings.Contains(extra, secret) {
			t.Fatalf("extra info leaked raw identifier or prompt: %q", extra)
		}
	}
}

func TestNativeCodeModeClassifiesOuterCallsAndPTCSubdispatch(t *testing.T) {
	cases := []struct {
		name, event, tool, engine string
		payload                   map[string]interface{}
	}{
		{"dsh ptc start", "ptc_dispatch_start", "read_file", "dsh-ptc", nil},
		{"dsh ptc settle", "ptc_dispatch", "read_file", "dsh-ptc", nil},
		{"dsh run_code", "tool_call", "run_code", "programmatic", nil},
		{"codex programmatic exec", "PreToolUse", "functions.exec", "programmatic", nil},
		{"claude code execution", "PreToolUse", "code_execution", "programmatic", nil},
		{"plugin run_code", "PreToolUse", "mcp__foo__run_code", "programmatic", nil},
		{"explicit engine", "tool_call", "run_code", "dsh-ptc", map[string]interface{}{"execution_mode": "code_mode", "code_mode_engine": "dsh-ptc"}},
		{"ordinary Bash", "PreToolUse", "Bash", "", nil},
		{"ordinary command", "PreToolUse", "exec_command", "", nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := nativeCodeModeEngine(tt.payload, tt.event, tt.tool); got != tt.engine {
				t.Fatalf("nativeCodeModeEngine() = %q; want %q", got, tt.engine)
			}
		})
	}
}

func TestNativeCodeModeDoesNotTreatSourceAsPathOrExposeIt(t *testing.T) {
	secret := "const token = 'SENSITIVE_CODE_MODE_SECRET'; await tools.read_file({path: '/private'});"
	payload := map[string]interface{}{
		"tool_name": "run_code",
		"tool_input": map[string]interface{}{
			"code":    secret,
			"command": "do not treat as path",
		},
	}
	input := payload["tool_input"].(map[string]interface{})
	if path := extractNativeCodeModePath(input); path != "" {
		t.Fatalf("source code or command leaked into Path: %q", path)
	}
	extra := buildNativeHookExtraInfo(payload, "PreToolUse", "run_code")
	for _, want := range []string{"execution_mode=code_mode", "execution_engine=programmatic", "code_digest=sha256:", "code_len="} {
		if !strings.Contains(extra, want) {
			t.Fatalf("missing %q in %q", want, extra)
		}
	}
	if strings.Contains(extra, secret) || strings.Contains(extra, "SENSITIVE_CODE_MODE_SECRET") || strings.Contains(extra, "/private") {
		t.Fatalf("unredacted source content: %q", extra)
	}
}

func TestNativeCodeModePTCSubcallsRetainOnlyHashedParent(t *testing.T) {
	payload := map[string]interface{}{
		"hook_event_name":     "ptc_dispatch_start",
		"tool_name":           "file_write",
		"parent_tool_call_id": "opaque-parent:run:1",
		"root_tool_call_id":   "opaque-root:run:1",
		"tool_call_id":        "opaque-parent:run:1:ptc:2",
	}
	extra := buildNativeHookExtraInfo(payload, "ptc_dispatch_start", "file_write")
	for _, want := range []string{"execution_mode=code_mode", "execution_engine=dsh-ptc", "execution_layer=nested", "parent_call_digest=sha256:", "root_call_digest=sha256:"} {
		if !strings.Contains(extra, want) {
			t.Fatalf("missing %q in %q", want, extra)
		}
	}
	if strings.Contains(extra, "opaque-parent") || strings.Contains(extra, "opaque-root") {
		t.Fatalf("raw opaque parent call ID leaked: %q", extra)
	}
	if !isNativeNestedCodeModeEvent("ptc_dispatch") || isNativeNestedCodeModeEvent("tool_call") {
		t.Fatal("only actual PTC dispatch events can skip global PID context replacement")
	}
}

func TestDshRelayOverflowIsExplicitMetadata(t *testing.T) {
	extra := buildNativeHookExtraInfo(map[string]interface{}{"dropped_events": float64(27)}, "telemetry_dropped", "")
	if !strings.Contains(extra, "hook_event=telemetry_dropped") || !strings.Contains(extra, "dropped_events=27") {
		t.Fatalf("dropped PTC hook events were not reported: %q", extra)
	}
}

func TestNativeCodeModeSubCallIDHashIsStableAndDoesNotLeakParent(t *testing.T) {
	original := "opaque-parent-secret:ptc:12"
	start := map[string]interface{}{"toolCallId": original}
	settled := map[string]interface{}{"tool_call_id": original}
	normalizeNativeNestedCallID(start, "ptc_dispatch_start")
	normalizeNativeNestedCallID(settled, "ptc_dispatch")
	startID, _ := start["tool_call_id"].(string)
	settleID, _ := settled["tool_call_id"].(string)
	if startID != settleID || !strings.HasPrefix(startID, "sha256:") || strings.Contains(startID, "opaque-parent") {
		t.Fatalf("PTC subcall IDs are not safe stable identifiers: start=%q settle=%q", startID, settleID)
	}
	ordinary := map[string]interface{}{"tool_call_id": original}
	normalizeNativeNestedCallID(ordinary, "tool_call")
	if ordinary["tool_call_id"] != original {
		t.Fatal("ordinary native hook call ID must preserve existing semantics")
	}
}

func TestNativeHookOutcomeRecordsOnlyErrorFlag(t *testing.T) {
	payload := map[string]interface{}{
		"tool_result": map[string]interface{}{"is_error": true, "content": "SENSITIVE_TOOL_RESULT"},
	}
	extra := buildNativeHookExtraInfo(payload, "ptc_dispatch", "read_file")
	if !strings.Contains(extra, "tool_is_error=true") || strings.Contains(extra, "SENSITIVE_TOOL_RESULT") {
		t.Fatalf("unexpected PTC result metadata: %q", extra)
	}
}
