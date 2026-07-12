package app

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRuntimeEventLogPathIsConfinedAndPrivate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	for _, raw := range []string{"events.jsonl", filepath.Join(root, "custom-events.jsonl")} {
		file, resolved, err := openRuntimeEventLogFileWithin(root, raw, os.O_CREATE|os.O_WRONLY|os.O_APPEND)
		if err != nil {
			t.Fatalf("openRuntimeEventLogFileWithin(%q) error = %v", raw, err)
		}
		if _, err := file.WriteString("event\n"); err != nil {
			t.Fatalf("WriteString() error = %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("runtime log permissions = %o, want 600", info.Mode().Perm())
		}
	}

	outside := filepath.Join(filepath.Dir(root), "outside.jsonl")
	if _, err := resolveRuntimeEventLogPathWithin(root, outside); err == nil {
		t.Fatal("outside runtime log path was accepted")
	}
	if _, err := resolveRuntimeEventLogPathWithin(root, filepath.Join("nested", "events.jsonl")); err == nil {
		t.Fatal("nested runtime log path was accepted")
	}
}

func TestRuntimeEventLogRejectsSymlinkAndNamedPipe(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Symlink(target, filepath.Join(root, "events.jsonl")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, _, err := openRuntimeEventLogFileWithin(root, "events.jsonl", os.O_WRONLY|os.O_TRUNC); err == nil {
		t.Fatal("symlink runtime log was accepted")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("symlink target changed: %q, %v", data, err)
	}

	fifo := filepath.Join(root, "events.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}
	if _, _, err := openRuntimeEventLogFileWithin(root, fifo, os.O_WRONLY|os.O_APPEND); err == nil {
		t.Fatal("named-pipe runtime log was accepted")
	} else if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected missing-file error: %v", err)
	}
}

func TestRuntimeEventLogRejectsHardlinkBeforeTruncation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Link(target, filepath.Join(root, "events.jsonl")); err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if _, _, err := openRuntimeEventLogFileWithin(root, "events.jsonl", os.O_WRONLY|os.O_TRUNC); err == nil {
		t.Fatal("hardlinked runtime log was accepted")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("hardlink target changed before rejection: %q, %v", data, err)
	}
}

func TestRuntimeEventLogRejectsSymlinkedRootWithoutCreatingOutside(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	outside := t.TempDir()
	linkedParent := filepath.Join(base, "linked")
	if err := os.Symlink(outside, linkedParent); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	root := filepath.Join(linkedParent, "agent-runtime")
	if _, _, err := openRuntimeEventLogFileWithin(root, "events.jsonl", os.O_CREATE|os.O_WRONLY); err == nil {
		t.Fatal("symlinked runtime log root was accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "agent-runtime")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime log setup created a directory outside its root: %v", err)
	}
}
