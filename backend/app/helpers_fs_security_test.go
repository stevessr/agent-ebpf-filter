package app

import (
	"path/filepath"
	"syscall"
	"testing"
)

func TestBuildFilePreviewRejectsNamedPipe(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "preview.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}
	if _, err := buildFilePreview(path); err == nil {
		t.Fatal("buildFilePreview() accepted a named pipe")
	}
}
