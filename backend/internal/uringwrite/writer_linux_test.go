//go:build linux && (amd64 || arm64)

package uringwrite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestLinuxIOUringABI(t *testing.T) {
	if got := unsafe.Sizeof(ringParams{}); got != 120 {
		t.Fatalf("io_uring_params size = %d, want 120", got)
	}
	if got := unsafe.Sizeof(sqEntry{}); got != 64 {
		t.Fatalf("io_uring_sqe size = %d, want 64", got)
	}
	if got := unsafe.Sizeof(cqEntry{}); got != 16 {
		t.Fatalf("io_uring_cqe size = %d, want 16", got)
	}
}

func TestIOUringRejectsNonAppendFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "no-append-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = New(file); err == nil {
		t.Fatal("New accepted a regular file without O_APPEND")
	}
}

func TestIOUringWritesInOrderAndLeavesFileOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer, err := New(file)
	if err != nil {
		t.Skipf("kernel, seccomp or filesystem does not permit io_uring: %v", err)
	}
	defer writer.Close()
	const count = 250
	for i := 0; i < count; i++ {
		if n, err := writer.Write([]byte("ordered-event\n")); err != nil || n != len("ordered-event\n") {
			t.Fatalf("write %d = (%d, %v)", i, n, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("should-fail")); !os.IsNotExist(err) && err != os.ErrClosed {
		t.Fatalf("Write after Close returned %v, want os.ErrClosed", err)
	}
	if _, err := file.Write([]byte("normal-path\n")); err != nil {
		t.Fatalf("Close unexpectedly closed caller's file: %v", err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("ordered-event\n", count) + "normal-path\n"
	if string(payload) != want {
		t.Fatalf("append order differs: got %d bytes, want %d", len(payload), len(want))
	}
}

func BenchmarkAppendWrite128KiB(b *testing.B) {
	benchmarkAppend(b, false)
}

func BenchmarkAppendIOUring128KiB(b *testing.B) {
	benchmarkAppend(b, true)
}

func benchmarkAppend(b *testing.B, useRing bool) {
	file, err := os.OpenFile(filepath.Join(b.TempDir(), "event-bench.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()
	var writer interface{ Write([]byte) (int, error) } = file
	if useRing {
		ring, err := New(file)
		if err != nil {
			b.Skipf("io_uring unavailable: %v", err)
		}
		defer ring.Close()
		writer = ring
	}
	payload := make([]byte, 128*1024)
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := writer.Write(payload); err != nil {
			b.Fatal(err)
		}
	}
}
