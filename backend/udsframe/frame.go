// Package udsframe implements the bounded length-prefixed framing shared by
// the backend UDS server and agent-wrapper client.
package udsframe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
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
	return ReadLimitInto(r, nil, maxPayloadSize)
}

// ReadLimitInto is ReadLimit reusing buf's storage when it is large enough.
// The returned payload aliases buf in that case, so a connection loop can
// hold one buffer for its lifetime instead of allocating per frame.
func ReadLimitInto(r io.Reader, buf []byte, maxPayloadSize int) ([]byte, error) {
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

	payload := buf[:0]
	if cap(payload) < int(size) {
		payload = make([]byte, int(size))
	} else {
		payload = payload[:size]
	}
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// Write sends one frame. On sockets the header and payload go out in a
// single gather write (writev) so the payload is never copied into a
// combined buffer; other writers receive two sequential writes.
func Write(w io.Writer, payload []byte) error {
	size := len(payload)
	if size == 0 || size > MaxPayloadSize {
		return fmt.Errorf("%w: %d (max %d)", ErrInvalidPayloadSize, size, MaxPayloadSize)
	}

	var header [HeaderSize]byte
	binary.BigEndian.PutUint32(header[:], uint32(size))
	if conn, ok := w.(net.Conn); ok {
		buffers := net.Buffers{header[:], payload}
		n, err := buffers.WriteTo(conn)
		if err != nil {
			return err
		}
		if n != int64(HeaderSize+size) {
			return io.ErrShortWrite
		}
		return nil
	}
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
