package main

import (
	"testing"

	"agent-ebpf-filter/pb"
)

func TestBuildProcessContextFromRegisterDefaultsRootPID(t *testing.T) {
	ctx := buildProcessContextFromRegister(registerPayload{
		PID:        321,
		ToolName:   "codex",
		AgentRunID: "run-1",
	})
	if ctx.RootAgentPid != 321 {
		t.Fatalf("RootAgentPid = %d, want 321", ctx.RootAgentPid)
	}
	if ctx.ToolName != "codex" {
		t.Fatalf("ToolName = %q, want codex", ctx.ToolName)
	}
	if ctx.ArgvDigest == "" {
		t.Fatal("ArgvDigest should be populated")
	}
}

func TestEnrichEventContextInheritsFromParentPID(t *testing.T) {
	trackedProcessContexts = newProcessContextStore()
	trackedProcessContexts.Set(100, processContext{
		RootAgentPid: 100,
		AgentRunID:   "run-42",
		ToolCallID:   "tool-7",
		TraceID:      "trace-9",
	})

	event := &pb.Event{Pid: 101, Ppid: 100, Type: "execve"}
	enrichEventContext(event)

	if event.RootAgentPid != 100 {
		t.Fatalf("RootAgentPid = %d, want 100", event.RootAgentPid)
	}
	if event.AgentRunId != "run-42" {
		t.Fatalf("AgentRunId = %q, want run-42", event.AgentRunId)
	}
	if event.ToolCallId != "tool-7" {
		t.Fatalf("ToolCallId = %q, want tool-7", event.ToolCallId)
	}
	if event.TraceId != "trace-9" {
		t.Fatalf("TraceId = %q, want trace-9", event.TraceId)
	}
	if _, ok := trackedProcessContexts.Get(101); !ok {
		t.Fatal("expected child PID context to be cached after enrichment")
	}
}

func TestEnrichEventContextMovesExecContext(t *testing.T) {
	trackedProcessContexts = newProcessContextStore()
	trackedProcessContexts.Set(200, processContext{RootAgentPid: 200, AgentRunID: "run-exec"})

	event := &pb.Event{Pid: 201, Type: "process_exec", ExtraInfo: "old_pid=200"}
	enrichEventContext(event)

	if event.AgentRunId != "run-exec" {
		t.Fatalf("AgentRunId = %q, want run-exec", event.AgentRunId)
	}
	if _, ok := trackedProcessContexts.Get(200); ok {
		t.Fatal("old pid context should be moved away")
	}
	if _, ok := trackedProcessContexts.Get(201); !ok {
		t.Fatal("new pid context should exist after exec move")
	}
}
