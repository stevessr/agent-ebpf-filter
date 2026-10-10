// Package collectorstream owns the kernel ring-buffer sample read loop.
// It never loads or attaches BPF programs, holds policy maps, or imports app.
// A separate privileged owner remains responsible for the reader lifecycle.
package collectorstream

import (
	"context"
	"errors"

	"github.com/cilium/ebpf/ringbuf"
)

// Reader permits a real ringbuf.Reader or an unprivileged test reader.
type Reader interface {
	ReadInto(*ringbuf.Record) error
	Close() error
}

var (
	ErrInvalidReader     = errors.New("kernel sample reader is nil")
	ErrInvalidConsumer   = errors.New("kernel sample consumer is nil")
	ErrInvalidSampleSize = errors.New("kernel sample size must be positive")
)

// Pump allocates a single initial sample buffer and reuses the record for the
// entire reader lifetime. The consume callback runs synchronously; it MUST NOT
// retain sample because ReadInto may overwrite it on the next iteration.
// It returns the reader's terminal error, including errors caused by Close.
func Pump(reader Reader, sampleSize int, consume func(sample []byte)) error {
	if reader == nil {
		return ErrInvalidReader
	}
	if consume == nil {
		return ErrInvalidConsumer
	}
	if sampleSize <= 0 {
		return ErrInvalidSampleSize
	}
	record := ringbuf.Record{RawSample: make([]byte, sampleSize)}
	for {
		if err := reader.ReadInto(&record); err != nil {
			return err
		}
		consume(record.RawSample)
	}
}

// CloseOnCancel unblocks a reader in Pump when the runtime context stops.
// Call it in a supervised goroutine alongside Pump to keep the prior lifecycle.
func CloseOnCancel(ctx context.Context, reader Reader) {
	if ctx == nil || reader == nil {
		return
	}
	<-ctx.Done()
	_ = reader.Close()
}
