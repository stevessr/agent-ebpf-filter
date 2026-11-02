//go:build linux

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/app/recording"
	"agent-ebpf-filter/pb"
)

func TestRuntimeEventLogWritevParityAndAuditChain(t *testing.T) {
	for _, mode := range []string{"buffered-write", "writev"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT_EVENT_LOG_IO", mode)
			path := filepath.Join(t.TempDir(), "events.jsonl")
			file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			writer, err := startRuntimeEventLogWriter(file)
			if err != nil {
				t.Fatal(err)
			}
			for i := 1; i <= 300; i++ {
				record := CapturedEventRecord{
					ReceivedAt: time.Unix(1700000000, int64(i)).UTC(),
					Event:      &pb.Event{Pid: uint32(i), Type: "openat", Path: "/tmp/test"},
				}
				if ok, err := writer.Enqueue(record); !ok || err != nil {
					t.Fatalf("enqueue %d: ok=%t err=%v", i, ok, err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := writer.FlushContext(ctx); err != nil {
				t.Fatal(err)
			}
			if err := writer.StopContext(ctx); err != nil {
				t.Fatal(err)
			}
			status := writer.Status()
			if status.IOBackend != mode || status.FailedTotal != 0 || status.DroppedTotal != 0 || status.PersistedTotal != 300 {
				t.Fatalf("unexpected status: %+v", status)
			}
			records, err := tailCapturedEventsFileAtRootContext(ctx, filepath.Dir(path), filepath.Base(path), 400)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 300 {
				t.Fatalf("got %d records; want 300", len(records))
			}
			if records[0].Event.GetPid() != 1 || records[299].Event.GetPid() != 300 {
				t.Fatalf("audit log lost event order")
			}
			if verification := recording.VerifyAuditChain(records); !verification.Valid || verification.CheckedRecords != 300 {
				t.Fatalf("audit verification: %+v", verification)
			}
		})
	}
}

func TestRuntimeEventLogWritevFallbackAndLargeRecord(t *testing.T) {
	t.Setenv("AGENT_EVENT_LOG_IO", "writev")
	fallback, err := os.CreateTemp(t.TempDir(), "no-append-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first, err := startRuntimeEventLogWriter(fallback)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status().IOBackend != "buffered-write" {
		t.Fatalf("expected buffered fallback: %+v", first.Status())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := first.StopContext(ctx); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "large-event.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := startRuntimeEventLogWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("X", 192*1024)
	if ok, err := writer.Enqueue(CapturedEventRecord{
		ReceivedAt: time.Unix(1700000000, 0).UTC(),
		Event:      &pb.Event{Pid: 9, Type: "openat", ExtraInfo: large},
	}); !ok || err != nil {
		t.Fatalf("enqueue large event: %t, %v", ok, err)
	}
	if err := writer.StopContext(ctx); err != nil {
		t.Fatal(err)
	}
	records, err := tailCapturedEventsFileAtRootContext(ctx, filepath.Dir(path), filepath.Base(path), 5)
	if err != nil || len(records) != 1 || records[0].Event.GetExtraInfo() != large {
		t.Fatalf("large payload truncated: n=%d err=%v", len(records), err)
	}
}
