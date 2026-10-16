package app

import (
	"agent-ebpf-filter/pb"
	"testing"
	"time"
)

func TestRenewSummaryPreservesHarnessSessionContext(t *testing.T) {
	summary, ok := buildRenewEventSummary(CapturedEventRecord{
		ReceivedAt: time.Now(),
		Event:      &pb.Event{Pid: 42, Ppid: 10, RootAgentPid: 10, Tag: "Claude Code", Comm: "curl", AgentRunId: "run-1", ConversationId: "session-1"},
	})
	if !ok || summary.RootAgentPID != 10 || summary.Tag != "Claude Code" || summary.AgentRunID != "run-1" || summary.ConversationID != "session-1" {
		t.Fatalf("compact summary lost harness/session metadata: %+v, ok=%v", summary, ok)
	}
}

func TestCodeCitySubagentLifecycleDigestIsScopedAndRedacted(t *testing.T) {
    digest := "0123456789abcdef"
    example := &pb.Event{
        Type: "native_hook",
        Comm: "SubagentStart:Explore",
        ExtraInfo: "hook_event=SubagentStart subagent_id_digest=sha256:" + digest + " prompt_len=45",
        RootAgentPid: 123,
        Pid: 123,
        AgentRunId: "run-123",
    }
    if got := nativeSubagentDigest(example); got != "sha256:0123456789abcdef" {
        t.Fatalf("short opaque digest missing: %q", got)
    }
    start, ok := buildRenewEventSummary(CapturedEventRecord{
        ReceivedAt: time.Now(), Event: example,
    })
    if !ok || start.SubagentID != "sha256:0123456789abcdef" {
        t.Fatalf("lifecycle digest absent from read-only compact summary: %+v", start)
    }
    stop := *example
    stop.Comm = "SubagentStop:Explore"
    if got := nativeSubagentDigest(&stop); got != start.SubagentID {
        t.Fatalf("start/stop identities should match: %q", got)
    }
    ordinary := *example
    ordinary.Type = "file_write"
    if got := nativeSubagentDigest(&ordinary); got != "" {
        t.Fatalf("file event incorrectly inherited subagent identity: %q", got)
    }
    ordinary.Type = "native_hook"
    ordinary.Comm = "PreToolUse:Read"
    if got := nativeSubagentDigest(&ordinary); got != "" {
        t.Fatalf("unrelated hook incorrectly inherited subagent identity: %q", got)
    }
    example.ExtraInfo = "subagent_id_digest=sha256:invalid"
    if got := nativeSubagentDigest(example); got != "" {
        t.Fatalf("malformed digest accepted: %q", got)
    }
}
