//go:build !linux || (!amd64 && !arm64)

package uringwrite

import (
	"errors"
	"os"
)

// Writer exists for cross-platform builds; io_uring is only implemented for
// Linux amd64/arm64. Callers should fall back to ordinary buffered writes.
type Writer struct{}

func New(_ *os.File) (*Writer, error) {
	return nil, errors.New("io_uring is unavailable on this platform")
}

func (*Writer) Write(_ []byte) (int, error) { return 0, os.ErrClosed }
func (*Writer) Close() error                { return nil }
