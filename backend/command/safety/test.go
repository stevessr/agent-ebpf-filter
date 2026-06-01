package main

import (
	"reflect"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestSplitCommandLinePreservesQuotedCommandArgument(t *testing.T) {
	got := splitCommandLine(`sudo bash -c "rm -rf /tmp/demo"`)
	want := []string{"sudo", "bash", "-c", "rm -rf /tmp/demo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitCommandLine() = %#v, want %#v", got, want)
	}
}

func TestCommandCandidateFromWrapperRecord(t *testing.T) {
	record := CapturedEventRecord{
		ReceivedAt: time.Unix(1700000000, 0).UTC(),
		Event: &pb.Event{
			Type:      "wrapper_intercept",
			EventType: pb.EventType_WRAPPER_INTERCEPT,
			Comm:      "rm",
			Path:      "rm -rf /tmp/demo",
			Behavior:  &pb.BehaviorClassification{PrimaryCategory: "FILE_DELETE"},
		},
	}

	got, ok := commandCandidateFromRecord(record, "memory")
	if !ok {
		t.Fatal("commandCandidateFromRecord() did not recognize wrapper event")
	}
	if got.Comm != "rm" || !reflect.DeepEqual(got.Args, []string{"-rf", "/tmp/demo"}) {
		t.Fatalf("candidate command = %q %#v", got.Comm, got.Args)
	}
	if got.Category != "FILE_DELETE" || got.Source != "memory" {
		t.Fatalf("candidate metadata = category %q source %q", got.Category, got.Source)
	}
}
