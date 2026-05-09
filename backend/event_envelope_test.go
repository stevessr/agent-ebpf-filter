package main

import (
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestNormalizeCapturedEventRecordBuildsWrapperEnvelope(t *testing.T) {
	record := normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.Unix(1710000000, 123).UTC(),
		Event: &pb.Event{
			Pid:        321,
			Ppid:       123,
			Type:       "wrapper_intercept",
			EventType:  pb.EventType_WRAPPER_INTERCEPT,
			Comm:       "git",
			Path:       "git status --short",
			AgentRunId: "run-1",
			TaskId:     "task-7",
			ToolCallId: "tool-3",
			ToolName:   "bash",
			TraceId:    "trace-8",
			Decision:   "ALERT",
			RiskScore:  88,
			Cwd:        "/workspace/demo",
		},
	})

	if record.Envelope == nil {
		t.Fatal("expected envelope to be populated")
	}
	if record.Envelope.GetSchemaVersion() != eventEnvelopeSchemaVersion {
		t.Fatalf("schema version = %q, want %q", record.Envelope.GetSchemaVersion(), eventEnvelopeSchemaVersion)
	}
	if record.Envelope.GetSource() != "wrapper" {
		t.Fatalf("source = %q, want wrapper", record.Envelope.GetSource())
	}
	if record.Envelope.GetTaskId() != "task-7" {
		t.Fatalf("task id = %q, want task-7", record.Envelope.GetTaskId())
	}
	if record.Envelope.GetCwd() != "/workspace/demo" {
		t.Fatalf("cwd = %q, want /workspace/demo", record.Envelope.GetCwd())
	}
	if record.Envelope.GetEventId() == "" {
		t.Fatal("expected deterministic event id")
	}
	wrapperPayload := record.Envelope.GetWrapperEvent()
	if wrapperPayload == nil {
		t.Fatal("expected wrapper payload")
	}
	if wrapperPayload.GetCommandLine() != "git status --short" {
		t.Fatalf("command line = %q, want git status --short", wrapperPayload.GetCommandLine())
	}
	if wrapperPayload.GetToolName() != "bash" {
		t.Fatalf("tool name = %q, want bash", wrapperPayload.GetToolName())
	}
}

func TestBuildCapturedEventJSONRecordsIncludesEnvelope(t *testing.T) {
	records := buildCapturedEventJSONRecords([]CapturedEventRecord{{
		ReceivedAt: time.Unix(1710000001, 0).UTC(),
		Event: &pb.Event{
			Pid:        99,
			Type:       "openat",
			EventType:  pb.EventType_OPENAT,
			Comm:       "python",
			Path:       "/workspace/app.py",
			AgentRunId: "run-json",
			TaskId:     "task-json",
			Cwd:        "/workspace",
		},
	}})

	if len(records) != 1 {
		t.Fatalf("json record count = %d, want 1", len(records))
	}
	envelope, ok := records[0]["Envelope"].(map[string]any)
	if !ok || envelope == nil {
		t.Fatalf("expected JSON envelope map, got %#v", records[0]["Envelope"])
	}
	if envelope["task_id"] != "task-json" {
		t.Fatalf("json task_id = %#v, want task-json", envelope["task_id"])
	}
	if envelope["cwd"] != "/workspace" {
		t.Fatalf("json cwd = %#v, want /workspace", envelope["cwd"])
	}
	filePayload, ok := envelope["file_event"].(map[string]any)
	if !ok || filePayload == nil {
		t.Fatalf("expected file_event payload, got %#v", envelope["file_event"])
	}
	if filePayload["path"] != "/workspace/app.py" {
		t.Fatalf("file_event.path = %#v, want /workspace/app.py", filePayload["path"])
	}
}
