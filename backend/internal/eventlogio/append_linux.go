//go:build linux

// Package eventlogio provides an optional vectored append sink for ordered
// JSONL audit logs. It avoids copying serialized records into a second batch
// buffer, but keeps them alive until the writev completion.
package eventlogio

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// AppendBatch has one owner goroutine, a bounded number of records and no
// concurrent I/O. Short writes are terminal: replaying an ambiguous append
// may duplicate or corrupt an audit-chain segment.
type AppendBatch struct {
	file    *os.File
	records [][]byte
	bytes   int
}

// NewAppendBatch rejects unsupported descriptors before the first write.
func NewAppendBatch(file *os.File, maxRecords int) (*AppendBatch, error) {
	if file == nil {
		return nil, errors.New("writev: nil log file")
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("writev requires a regular file")
	}
	flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
	if err != nil {
		return nil, fmt.Errorf("writev fcntl: %w", err)
	}
	if flags&unix.O_APPEND == 0 {
		return nil, errors.New("writev requires O_APPEND")
	}
	if maxRecords <= 0 || maxRecords > 1024 {
		return nil, errors.New("writev invalid maximum batch record count")
	}
	return &AppendBatch{file: file, records: make([][]byte, 0, maxRecords)}, nil
}

// AddLine takes ownership of payload and appends the JSONL delimiter. The
// upstream JSON marshaler returns a fresh buffer for each record.
func (b *AppendBatch) AddLine(payload []byte) error {
	if b == nil || len(b.records) >= cap(b.records) {
		return errors.New("writev batch is full or nil")
	}
	line := append(payload, '\n')
	b.records = append(b.records, line)
	b.bytes += len(line)
	return nil
}

func (b *AppendBatch) Bytes() int {
	if b == nil {
		return 0
	}
	return b.bytes
}

func (b *AppendBatch) Flush() error {
	if b == nil || len(b.records) == 0 {
		return nil
	}
	// Keep a single syscall and one append operation per batch; do not retry
	// a short write, which may have left an incomplete JSONL tail.
	n, err := unix.Writev(int(b.file.Fd()), b.records)
	if err != nil {
		return fmt.Errorf("writev append: %w", err)
	}
	if n != b.bytes {
		return fmt.Errorf("writev append: %w: wrote %d of %d bytes", io.ErrShortWrite, n, b.bytes)
	}
	for i := range b.records {
		b.records[i] = nil
	}
	b.records = b.records[:0]
	b.bytes = 0
	return nil
}
