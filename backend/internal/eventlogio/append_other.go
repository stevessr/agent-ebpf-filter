//go:build !linux

package eventlogio

import (
	"errors"
	"os"
)

// The writev backend is optional. Unsupported targets retain bufio writes.
type AppendBatch struct{}

func NewAppendBatch(_ *os.File, _ int) (*AppendBatch, error) {
	return nil, errors.New("writev append is only available on Linux")
}
func (*AppendBatch) AddLine(_ []byte) error { return errors.New("writev append unavailable") }
func (*AppendBatch) Bytes() int             { return 0 }
func (*AppendBatch) Flush() error           { return errors.New("writev append unavailable") }
