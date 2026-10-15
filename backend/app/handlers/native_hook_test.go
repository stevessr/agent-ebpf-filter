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
		"agent_id": "private-agent-id-123",
		"agent_type": "Explore",
		"prompt": "private user prompt",
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
