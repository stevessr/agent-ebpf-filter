package collectorstream

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"
)

type sequenceReader struct {
	samples [][]byte
	count   int
}

func (r *sequenceReader) ReadInto(record *ringbuf.Record) error {
	if r.count >= len(r.samples) {
		return io.EOF
	}
	sample := r.samples[r.count]
	r.count++
	record.RawSample = append(record.RawSample[:0], sample...)
	return nil
}
func (r *sequenceReader) Close() error { return nil }

func TestPumpPreservesBufferReuseAndOrder(t *testing.T) {
	reader := &sequenceReader{samples: [][]byte{{1, 2}, {3, 4}}}
	var values [][]byte
	var addresses []*byte
	err := Pump(reader, 8, func(sample []byte) {
		values = append(values, append([]byte(nil), sample...))
		addresses = append(addresses, &sample[0])
	})
	if !errors.Is(err, io.EOF) {
		t.Fatalf("terminal error = %v", err)
	}
	if len(values) != 2 || values[0][0] != 1 || values[1][0] != 3 {
		t.Fatalf("sample ordering wrong: %v", values)
	}
	if addresses[0] != addresses[1] {
		t.Fatal("read loop unexpectedly allocated a new sample buffer")
	}
	if reader.count != 2 {
		t.Fatalf("read count = %d", reader.count)
	}
}

func TestPumpRejectsInvalidInputs(t *testing.T) {
	reader := &sequenceReader{}
	for _, tc := range []struct {
		err error
		got error
	}{
		{ErrInvalidReader, Pump(nil, 1, func([]byte) {})},
		{ErrInvalidConsumer, Pump(reader, 1, nil)},
		{ErrInvalidSampleSize, Pump(reader, 0, func([]byte) {})},
	} {
		if !errors.Is(tc.got, tc.err) {
			t.Fatalf("got %v, want %v", tc.got, tc.err)
		}
	}
}

type blockingReader struct {
	closed chan struct{}
	once sync.Once
}

func (r *blockingReader) ReadInto(_ *ringbuf.Record) error {
	<-r.closed
	return io.EOF
}

func (r *blockingReader) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

func TestCloseOnCancelUnblocksPump(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := &blockingReader{closed: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- Pump(reader, 32, func([]byte) {}) }()
	closeDone := make(chan struct{})
	go func() {
		CloseOnCancel(ctx, reader)
		close(closeDone)
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("pump stopped with %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pump not stopped by reader Close")
	}
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("cancellation listener did not exit")
	}
}
