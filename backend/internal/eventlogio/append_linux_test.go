//go:build linux

package eventlogio

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendBatchPreservesOrderAndFlushes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	batch, err := NewAppendBatch(file, 128)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.Flush(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 128; i++ {
		if err := batch.AddLine([]byte("{\"id\":\"event\"}")); err != nil {
			t.Fatal(err)
		}
	}
	if err := batch.AddLine([]byte("overflow")); err == nil {
		t.Fatal("batch accepted more vectors than reserved")
	}
	if err := batch.Flush(); err != nil {
		t.Fatal(err)
	}
	if batch.Bytes() != 0 {
		t.Fatalf("remaining bytes after Flush: %d", batch.Bytes())
	}
	if err := batch.AddLine([]byte("{\"id\":\"final\"}")); err != nil {
		t.Fatal(err)
	}
	if err := batch.Flush(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("{\"id\":\"event\"}\n", 128) + "{\"id\":\"final\"}\n"
	if string(data) != want {
		t.Fatalf("writev bytes differ: got %d, want %d", len(data), len(want))
	}
}

func TestAppendBatchRejectsIncompatibleDescriptor(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "events-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := NewAppendBatch(file, 128); err == nil {
		t.Fatal("expected rejection of descriptor without O_APPEND")
	}
	if _, err := NewAppendBatch(nil, 128); err == nil {
		t.Fatal("nil descriptor accepted")
	}
	if _, err := NewAppendBatch(file, 1025); err == nil {
		t.Fatal("batch larger than vector limit accepted")
	}
}

func TestAppendBatchClosedFileIsTerminal(t *testing.T) {
	file, err := os.OpenFile(filepath.Join(t.TempDir(), "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := NewAppendBatch(file, 128)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.AddLine([]byte("record")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := batch.Flush(); err == nil {
		t.Fatal("Flush succeeded with a closed file")
	}
	if batch.Bytes() == 0 {
		t.Fatal("failed batch discarded without explicit error handling")
	}
}

func benchmarkBatch(b *testing.B, backend string, count, size int) {
	file, err := os.OpenFile(filepath.Join(b.TempDir(), "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()
	payload := strings.Repeat("x", size-1)
	lines := make([][]byte, count)
	for i := range lines {
		lines[i] = make([]byte, size-1, size)
		copy(lines[i], payload)
	}
	batch, err := NewAppendBatch(file, count)
	if err != nil {
		b.Fatal(err)
	}
	writer := bufio.NewWriterSize(file, 256*1024)
	b.SetBytes(int64(size * count))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		switch backend {
		case "writev":
			for _, line := range lines {
				if err := batch.AddLine(line); err != nil {
					b.Fatal(err)
				}
			}
			if err := batch.Flush(); err != nil {
				b.Fatal(err)
			}
		case "buffered":
			for _, line := range lines {
				if _, err := writer.Write(line); err != nil {
					b.Fatal(err)
				}
				if err := writer.WriteByte('\n'); err != nil {
					b.Fatal(err)
				}
			}
			if err := writer.Flush(); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkBuffered128x512(b *testing.B)  { benchmarkBatch(b, "buffered", 128, 512) }
func BenchmarkWritev128x512(b *testing.B)    { benchmarkBatch(b, "writev", 128, 512) }
func BenchmarkBuffered128x2048(b *testing.B) { benchmarkBatch(b, "buffered", 128, 2048) }
func BenchmarkWritev128x2048(b *testing.B)   { benchmarkBatch(b, "writev", 128, 2048) }
