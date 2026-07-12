// Package udsframe implements the bounded length-prefixed framing shared by
// the backend UDS server and agent-wrapper client.
package udsframe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	HeaderSize     = 4
	MaxPayloadSize = 4 << 20
)

var ErrInvalidPayloadSize = errors.New("invalid UDS frame payload size")

func Read(r io.Reader) ([]byte, error) {
	return ReadLimit(r, MaxPayloadSize)
}

// ReadLimit reads one frame while enforcing a caller-specific payload limit.
// The limit may narrow, but never expand, the protocol-wide maximum.
func ReadLimit(r io.Reader, maxPayloadSize int) ([]byte, error) {
	if maxPayloadSize <= 0 || maxPayloadSize > MaxPayloadSize {
		return nil, fmt.Errorf("%w: invalid read limit %d (protocol max %d)", ErrInvalidPayloadSize, maxPayloadSize, MaxPayloadSize)
	}
	var header [HeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}

	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > uint32(maxPayloadSize) {
		return nil, fmt.Errorf("%w: %d (max %d)", ErrInvalidPayloadSize, size, maxPayloadSize)
	}

	payload := make([]byte, int(size))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func Write(w io.Writer, payload []byte) error {
	size := len(payload)
	if size == 0 || size > MaxPayloadSize {
		return fmt.Errorf("%w: %d (max %d)", ErrInvalidPayloadSize, size, MaxPayloadSize)
	}

	var header [HeaderSize]byte
	binary.BigEndian.PutUint32(header[:], uint32(size))
	if err := writeFull(w, header[:]); err != nil {
		return err
	}
	return writeFull(w, payload)
}

func writeFull(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(payload) {
			return io.ErrShortWrite
		}
		payload = payload[n:]
	}
	return nil
}
