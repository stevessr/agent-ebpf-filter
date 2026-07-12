package app

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSignalProgramLogPathIsConfinedAndPrivate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	selected := SelectedProgramSignalLog{Program: "codex", Path: "custom.pb.gzlog"}
	file, resolved, err := openSignalProgramLogWithin(root, selected, os.O_CREATE|os.O_WRONLY|os.O_APPEND)
	if err != nil {
		t.Fatalf("openSignalProgramLogWithin() error = %v", err)
	}
	if _, err := file.WriteString("frame"); err != nil {
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
		t.Fatalf("signal program log permissions = %o, want 600", info.Mode().Perm())
	}

	for _, path := range []string{
		filepath.Join(filepath.Dir(root), "outside.pb.gzlog"),
		filepath.Join("nested", "log.pb.gzlog"),
		"../outside.pb.gzlog",
	} {
		if _, err := resolveSignalProgramLogPathWithin(root, SelectedProgramSignalLog{Program: "codex", Path: path}); err == nil {
			t.Fatalf("unsafe signal program log path %q was accepted", path)
		}
	}
}

func TestSignalProgramLogRejectsSymlinkHardlinkAndNamedPipe(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	symlinkName := "symlink.pb.gzlog"
	if err := os.Symlink(target, filepath.Join(root, symlinkName)); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, _, err := openSignalProgramLogWithin(root, SelectedProgramSignalLog{Path: symlinkName}, os.O_WRONLY|os.O_APPEND); err == nil {
		t.Fatal("symlink signal program log was accepted")
	}

	hardlinkName := "hardlink.pb.gzlog"
	if err := os.Link(target, filepath.Join(root, hardlinkName)); err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if _, _, err := openSignalProgramLogWithin(root, SelectedProgramSignalLog{Path: hardlinkName}, os.O_WRONLY|os.O_APPEND); err == nil {
		t.Fatal("hardlink signal program log was accepted")
	}

	fifoName := "events.fifo"
	if err := syscall.Mkfifo(filepath.Join(root, fifoName), 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}
	if _, _, err := openSignalProgramLogWithin(root, SelectedProgramSignalLog{Path: fifoName}, os.O_WRONLY|os.O_APPEND); err == nil {
		t.Fatal("named-pipe signal program log was accepted")
	} else if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected missing-file error: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("outside target changed: %q, %v", data, err)
	}
}

func TestSignalProgramLogRejectsSymlinkedRootWithoutCreatingOutside(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	outside := t.TempDir()
	linkedParent := filepath.Join(base, "linked")
	if err := os.Symlink(outside, linkedParent); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	root := filepath.Join(linkedParent, "program-logs")
	if _, _, err := openSignalProgramLogWithin(root, SelectedProgramSignalLog{Path: "codex.pb.gzlog"}, os.O_CREATE|os.O_WRONLY); err == nil {
		t.Fatal("symlinked signal program log root was accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "program-logs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("signal program log setup created a directory outside its root: %v", err)
	}
}
