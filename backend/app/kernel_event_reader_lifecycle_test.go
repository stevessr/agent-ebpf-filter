package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"
)

type blockingKernelEventReader struct {
	closed    chan struct{}
	closeOnce sync.Once
}

func newBlockingKernelEventReader() *blockingKernelEventReader {
	return &blockingKernelEventReader{closed: make(chan struct{})}
}

func (r *blockingKernelEventReader) ReadInto(_ *ringbuf.Record) error {
	<-r.closed
	return errors.New("reader closed")
}

func (r *blockingKernelEventReader) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

func TestKernelEventReaderClosesAndJoinsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	jobs := &runtimeBackgroundJobs{}
	reader := newBlockingKernelEventReader()
	startKernelEventReader(ctx, reader, jobs)

	cancel()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := jobs.Wait(waitCtx); err != nil {
		t.Fatalf("kernel event reader did not stop: %v", err)
	}
	select {
	case <-reader.closed:
	default:
		t.Fatal("kernel event reader was not closed")
	}
}
