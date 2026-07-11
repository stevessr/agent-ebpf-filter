package app

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-ebpf-filter/pb"
	"agent-ebpf-filter/redaction"
)

func TestRecordCapturedEventRedactsBeforeArchiveAndPersistence(t *testing.T) {
	secret := "sk-abcdefghijklmnopqrstuvwxyz0123456789"
	path := filepath.Join(t.TempDir(), "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}

	oldRuntime := runtimeSettingsStore
	oldArchive := capturedEventArchive
	oldEngine := globalRedactionEngine
	runtimeSettingsStore = &runtimeState{
		settings:  RuntimeSettings{LogPersistenceEnabled: true, LogFilePath: path},
		logFile:   file,
		logWriter: bufio.NewWriter(file),
	}
	capturedEventArchive = newEventArchive(10)
	globalRedactionEngine = redaction.NewRedactionEngine(redaction.RedactionPolicy{
		Level:              redaction.RedactionLevelStrict,
		DefaultPlaceholder: "[REDACTED]",
	})
	t.Cleanup(func() {
		runtimeSettingsStore.mu.Lock()
		runtimeSettingsStore.closeLogWriterLocked()
		runtimeSettingsStore.mu.Unlock()
		runtimeSettingsStore = oldRuntime
		capturedEventArchive = oldArchive
		globalRedactionEngine = oldEngine
	})

	record := recordCapturedEvent(&pb.Event{
		Type:      "openat",
		Comm:      "codex",
		Path:      "/home/user/private.txt",
		ExtraInfo: "authorization=" + secret,
	})
	assertCapturedRecordRedacted(t, record, secret)

	archived := capturedEventArchive.Snapshot(1)
	if len(archived) != 1 {
		t.Fatalf("archive length = %d, want 1", len(archived))
	}
	assertCapturedRecordRedacted(t, archived[0], secret)

	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(persisted), secret) {
		t.Fatal("persisted event log contains the raw secret")
	}
}

func assertCapturedRecordRedacted(t *testing.T, record CapturedEventRecord, secret string) {
	t.Helper()
	if record.Event == nil || record.Envelope == nil || record.Envelope.GetLegacyEvent() == nil {
		t.Fatal("captured record is incomplete")
	}
	values := []string{
		record.Event.GetExtraInfo(),
		record.Envelope.GetLegacyEvent().GetExtraInfo(),
	}
	for _, value := range values {
		if strings.Contains(value, secret) {
			t.Fatalf("captured record contains raw secret: %q", value)
		}
	}
}

func TestRedactCapturedEventRecordCoversEnvelopePayloads(t *testing.T) {
	t.Parallel()
	secret := "sk-abcdefghijklmnopqrstuvwxyz0123456789"
	engine := redaction.NewRedactionEngine(redaction.RedactionPolicy{
		Level:              redaction.RedactionLevelStrict,
		DefaultPlaceholder: "[REDACTED]",
	})

	events := []*pb.Event{
		{Type: "network_connect", NetEndpoint: "secret.example:443", ExtraInfo: secret, Sni: "secret.example"},
		{Type: "process_fork", ExtraInfo: "child_pid=2 token=" + secret},
		{Type: "wrapper_intercept", Path: "curl -H Authorization:" + secret, ExtraInfo: secret},
		{Type: "native_hook", Comm: "hook:codex", Path: "/home/private", ExtraInfo: secret},
		{Type: "mcp_call", ToolName: "filesystem", NetEndpoint: "secret.example:443", ExtraInfo: secret},
		{Type: "system_metric", ExtraInfo: `{"alert":"` + secret + `"}`},
		{Type: "otel_span", ToolName: "request", ExtraInfo: `{"error":"` + secret + `"}`},
	}

	for _, event := range events {
		event := event
		t.Run(event.GetType(), func(t *testing.T) {
			record := normalizeCapturedEventRecord(CapturedEventRecord{Event: cloneProtoEvent(event)})
			record = redactCapturedEventRecord(record, engine)
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("redacted %s envelope contains raw secret: %s", event.GetType(), encoded)
			}
		})
	}
}
