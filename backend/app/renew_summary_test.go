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
