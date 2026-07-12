package app

import (
	"agent-ebpf-filter/pb"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sys/unix"
)

func TestEventRecordingWritesAndReadsConfinedJSONL(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	store := &eventRecordingState{}
	status, err := store.startAtRoot(root, "events.jsonl", true)
	if err != nil {
		t.Fatalf("startAtRoot() error = %v", err)
	}
	defer store.Stop()
	if !status.Active || status.Path != filepath.Join(root, "events.jsonl") {
		t.Fatalf("unexpected recording status %#v", status)
	}
	store.Record(CapturedEventRecord{
		ReceivedAt: time.Unix(1710000000, 0).UTC(),
		Event:      &pb.Event{Pid: 42, Ppid: 1, Comm: "codex", Type: "openat", Path: "/tmp/demo"},
	})
	status, err = store.Stop()
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if status.Count != 1 {
		t.Fatalf("recorded count = %d, want 1", status.Count)
	}
	info, err := os.Stat(status.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("recording mode = %o, want 600", got)
	}
	records, resolved, err := readCapturedEventsFileAtRoot(root, status.Path, 100)
	if err != nil {
		t.Fatalf("readCapturedEventsFileAtRoot() error = %v", err)
	}
	if resolved != status.Path || len(records) != 1 || records[0].Event.GetPid() != 42 || records[0].Envelope == nil {
		t.Fatalf("unexpected replay path=%q records=%#v", resolved, records)
	}
}

func TestRecordingPathsRejectEscapeAndNestedDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	store := &eventRecordingState{}
	for _, path := range []string{"../escape.jsonl", "nested/events.jsonl", filepath.Join(filepath.Dir(root), "outside.jsonl")} {
		if _, err := store.startAtRoot(root, path, false); !errors.Is(err, errRecordingPathOutsideRoot) {
			t.Fatalf("path %q error = %v, want confinement error", path, err)
		}
	}
}

func TestRecordingOperationsRejectSymlinksAndHardlinks(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "recordings")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside")
	if err := os.WriteFile(outside, []byte("do-not-change"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "events.jsonl")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Fatal(err)
	}
	store := &eventRecordingState{}
	for _, truncate := range []bool{false, true} {
		if _, err := store.startAtRoot(root, "events.jsonl", truncate); err == nil {
			t.Fatalf("truncate=%t unexpectedly accepted symlink", truncate)
		}
	}
	if _, _, err := readCapturedEventsFileAtRoot(root, "events.jsonl", 10); err == nil {
		t.Fatal("replay unexpectedly accepted symlink")
	}
	if _, _, err := saveBrowserRecordingExportAtRoot(root, "events.jsonl", json.RawMessage(`{"snapshots":[]}`)); err == nil {
		t.Fatal("browser save unexpectedly accepted symlink")
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "do-not-change" {
		t.Fatalf("outside target changed: data=%q err=%v", data, err)
	}
	if err := os.Remove(symlink); err != nil {
		t.Fatal(err)
	}
	hardlink := filepath.Join(root, "hardlink.jsonl")
	if err := os.Link(outside, hardlink); err != nil {
		t.Fatal(err)
	}
	if _, err := store.startAtRoot(root, "hardlink.jsonl", false); err == nil {
		t.Fatal("append unexpectedly accepted hardlink")
	}
}

func TestOpenRecordingsRootRejectsSymlinkRoot(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "recordings")
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	if file, _, err := openRecordingsRoot(root); err == nil {
		_ = file.Close()
		t.Fatal("symlink recordings root unexpectedly accepted")
	}
}

func TestOpenRecordingsRootDoesNotTraverseSymlinkedParent(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(base, "linked-parent")
	if err := os.Symlink(outside, linkedParent); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(linkedParent, "recordings")
	if file, _, err := openRecordingsRoot(root); err == nil {
		_ = file.Close()
		t.Fatal("symlinked parent unexpectedly accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "recordings")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("secure root creation escaped through symlink: %v", err)
	}
}

func TestReplayRejectsSpecialAndOversizedFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(root, "events.fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readCapturedEventsFileAtRoot(root, "events.fifo", 10); err == nil {
		t.Fatal("replay unexpectedly accepted FIFO")
	}
	large := filepath.Join(root, "large.jsonl")
	file, err := os.OpenFile(large, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(eventReplayMaxFileBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	_ = file.Close()
	if _, _, err := readCapturedEventsFileAtRoot(root, "large.jsonl", 10); !errors.Is(err, errRecordingFileTooLarge) {
		t.Fatalf("oversized replay error = %v", err)
	}
}

func TestSaveBrowserRecordingExportIsConfinedAndBounded(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	payload := json.RawMessage(`{"version":1,"snapshots":[{"recordedAt":"now","graph":{"eventCount":1,"source":"browser_memory","nodes":[],"edges":[]}}]}`)
	gotPath, snapshots, err := saveBrowserRecordingExportAtRoot(root, "browser-memory.json", payload)
	if err != nil {
		t.Fatalf("saveBrowserRecordingExportAtRoot() error = %v", err)
	}
	if gotPath != filepath.Join(root, "browser-memory.json") || snapshots != 1 {
		t.Fatalf("path=%q snapshots=%d", gotPath, snapshots)
	}
	data, err := os.ReadFile(gotPath)
	if err != nil || !json.Valid(data) {
		t.Fatalf("saved payload invalid: err=%v data=%q", err, data)
	}
	info, err := os.Stat(gotPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("saved mode=%v err=%v", info.Mode(), err)
	}
	oversized := json.RawMessage(bytes.Repeat([]byte(" "), browserRecordingExportMaxBytes+1))
	if _, _, err := saveBrowserRecordingExportAtRoot(root, "large.json", oversized); !errors.Is(err, errBrowserRecordingTooLarge) {
		t.Fatalf("oversized browser export error = %v", err)
	}
}

func TestHandleSaveBrowserRecordingRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &spaceReader{remaining: browserRecordingRequestMaxBytes + 1}
	req := httptest.NewRequest(http.MethodPost, "/events/recording/browser/save", reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req
	handleSaveBrowserRecording(ctx)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

type spaceReader struct {
	remaining int64
}

func (reader *spaceReader) Read(buffer []byte) (int, error) {
	if reader.remaining <= 0 {
		return 0, errors.New("unexpected end")
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	for index := range buffer {
		buffer[index] = ' '
	}
	reader.remaining -= int64(len(buffer))
	return len(buffer), nil
}

func TestResolveRecordingTargetAcceptsAbsoluteFileWithinRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	want := filepath.Join(root, "events.jsonl")
	name, path, err := resolveRecordingTarget(root, want, "")
	if err != nil || name != "events.jsonl" || path != want {
		t.Fatalf("name=%q path=%q err=%v", name, path, err)
	}
	if strings.Contains(name, string(os.PathSeparator)) {
		t.Fatalf("resolved name contains separator: %q", name)
	}
}
