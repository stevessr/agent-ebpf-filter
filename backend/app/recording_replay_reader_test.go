package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestReplayTailReaderReturnsLastValidRecordsInOrder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writeRecord := func(pid uint32, extra string, newline bool) {
		t.Helper()
		payload, err := json.Marshal(CapturedEventRecord{
			ReceivedAt: time.Unix(int64(pid), 0).UTC(),
			Event:      &pb.Event{Pid: pid, Comm: "codex", Type: "read", ExtraInfo: extra},
		})
		if err != nil {
			t.Fatal(err)
		}
		if newline {
			payload = append(payload, '\n')
		}
		if _, err := file.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	writeRecord(1, "old", true)
	writeRecord(2, strings.Repeat("x", eventReplayReadChunkBytes+1024), true)
	if _, err := file.WriteString("not-json\n\n"); err != nil {
		t.Fatal(err)
	}
	writeRecord(3, "new", false)
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	records, resolved, err := readCapturedEventsFileAtRootContext(context.Background(), root, "events.jsonl", 2)
	if err != nil {
		t.Fatalf("readCapturedEventsFileAtRootContext() error = %v", err)
	}
	if resolved != path || len(records) != 2 || records[0].Event.GetPid() != 2 || records[1].Event.GetPid() != 3 {
		t.Fatalf("unexpected replay tail path=%q records=%v", resolved, replayRecordPIDs(records))
	}
	for _, record := range records {
		if record.Envelope == nil {
			t.Fatalf("record %d was not normalized", record.Event.GetPid())
		}
	}
}

func TestReplayTailReaderDoesNotScanUnneededPrefix(t *testing.T) {
	payload, err := json.Marshal(CapturedEventRecord{
		ReceivedAt: time.Unix(3, 0).UTC(),
		Event:      &pb.Event{Pid: 3, Comm: "codex", Type: "read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("not-json\n"), 100_000)
	data = append(data, payload...)
	reader := &countingReaderAt{ReaderAt: bytes.NewReader(data)}
	records, err := readCapturedEventTail(context.Background(), reader, int64(len(data)), 1)
	if err != nil {
		t.Fatalf("readCapturedEventTail() error = %v", err)
	}
	if len(records) != 1 || records[0].Event.GetPid() != 3 {
		t.Fatalf("unexpected replay records %v", replayRecordPIDs(records))
	}
	if readBytes := reader.ReadBytes(); readBytes > eventReplayReadChunkBytes {
		t.Fatalf("tail replay read %d bytes for one latest record, want at most one %d-byte block", readBytes, eventReplayReadChunkBytes)
	}
}

func TestReplayTailReaderHonorsCancellationDuringMalformedTail(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("not-json\n"), 10000)
	if err := os.WriteFile(filepath.Join(root, "events.jsonl"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterErrChecksContext{Context: context.Background(), cancelAfter: 6}
	if _, _, err := readCapturedEventsFileAtRootContext(ctx, root, "events.jsonl", 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled replay error = %v, want context.Canceled", err)
	}
}

func TestReplayTailReaderRejectsOversizedLineAndMalformedLineFlood(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "events.jsonl")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), eventReplayMaxLineBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readCapturedEventsFileAtRoot(root, "events.jsonl", 10); !errors.Is(err, errRecordingLineTooLarge) {
		t.Fatalf("oversized line error = %v", err)
	}

	flood := bytes.Repeat([]byte("x\n"), eventReplayMaxScannedLines)
	flood = append(flood, 'x')
	if err := os.WriteFile(path, flood, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readCapturedEventsFileAtRoot(root, "events.jsonl", 10); !errors.Is(err, errRecordingTooManyLines) {
		t.Fatalf("malformed line flood error = %v", err)
	}
}

func replayRecordPIDs(records []CapturedEventRecord) []uint32 {
	pids := make([]uint32, 0, len(records))
	for _, record := range records {
		pids = append(pids, record.Event.GetPid())
	}
	return pids
}

type cancelAfterErrChecksContext struct {
	context.Context
	mu          sync.Mutex
	checks      int
	cancelAfter int
}

func (c *cancelAfterErrChecksContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks++
	if c.checks >= c.cancelAfter {
		return context.Canceled
	}
	return nil
}

type countingReaderAt struct {
	io.ReaderAt
	mu        sync.Mutex
	readBytes int
}

func (r *countingReaderAt) ReadAt(buffer []byte, offset int64) (int, error) {
	read, err := r.ReaderAt.ReadAt(buffer, offset)
	r.mu.Lock()
	r.readBytes += read
	r.mu.Unlock()
	return read, err
}

func (r *countingReaderAt) ReadBytes() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.readBytes
}
