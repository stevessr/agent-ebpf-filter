package main

import (
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestEventRecordingWritesAndReadsJSONL(t *testing.T) {
	path := t.TempDir() + "/events.jsonl"
	status, err := eventRecordingStore.Start(path, true)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer eventRecordingStore.Stop()
	if !status.Active {
		t.Fatalf("recording should be active")
	}
	eventRecordingStore.Record(CapturedEventRecord{
		ReceivedAt: time.Unix(1710000000, 0).UTC(),
		Event:      &pb.Event{Pid: 42, Ppid: 1, Comm: "codex", Type: "openat", Path: "/tmp/demo"},
	})
	status, err = eventRecordingStore.Stop()
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if status.Count != 1 {
		t.Fatalf("recorded count = %d, want 1", status.Count)
	}

	records, err := readCapturedEventsFile(path, 100)
	if err != nil {
		t.Fatalf("readCapturedEventsFile() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	if records[0].Event.GetPid() != 42 || records[0].Envelope == nil {
		t.Fatalf("unexpected replay record %#v", records[0])
	}
}
