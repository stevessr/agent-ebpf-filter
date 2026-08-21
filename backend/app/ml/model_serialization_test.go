package ml

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteModelFileAtomicSupportsConcurrentWriters(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "model.bin")
	payloads := [][]byte{
		bytes.Repeat([]byte("a"), 4096),
		bytes.Repeat([]byte("b"), 8192),
		bytes.Repeat([]byte("c"), 16384),
		bytes.Repeat([]byte("d"), 32768),
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(payloads))
	for _, payload := range payloads {
		payload := payload
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- writeModelFileAtomic(path, payload, 0o600)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("writeModelFileAtomic() error = %v", err)
		}
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, payload := range payloads {
		if bytes.Equal(got, payload) {
			return
		}
	}
	t.Fatalf("final model is not one complete writer payload: %d bytes", len(got))
}
