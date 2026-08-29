package tls

import (
	"encoding/binary"
	"fmt"
)

const (
	bpfTSOpenSSLMetadataSize = 48
	bpfTSOpenSSLSampleSize   = 4096
	bpfTSOpenSSLEventSize    = bpfTSOpenSSLMetadataSize + bpfTSOpenSSLSampleSize

	bpfTSOpenSSLDirectionRecv = 0
	bpfTSOpenSSLDirectionSend = 1
	bpfTSOpenSSLRingName      = "tlsOpenSSLEvents"
)

type bpfTSOpenSSLEvent struct {
	TimestampNS  uint64
	ConnectionID uint64
	PID          uint32 // TGID from bpf.pid()
	TID          uint32 // host TID from bpf.tid()
	Length       int32
	Direction    uint8
	Function     uint8
	Comm         [16]byte
	Sample       [bpfTSOpenSSLSampleSize]byte
}

func decodeBpfTSOpenSSLEvent(raw []byte) (bpfTSOpenSSLEvent, error) {
	if len(raw) != bpfTSOpenSSLEventSize {
		return bpfTSOpenSSLEvent{}, fmt.Errorf(
			"bpf-ts OpenSSL event size mismatch: got %d bytes, want %d",
			len(raw), bpfTSOpenSSLEventSize,
		)
	}

	reserved := binary.LittleEndian.Uint16(raw[30:32])
	if reserved != 0 {
		return bpfTSOpenSSLEvent{}, fmt.Errorf("bpf-ts OpenSSL reserved field is non-zero: %#x", reserved)
	}

	event := bpfTSOpenSSLEvent{
		TimestampNS:  binary.LittleEndian.Uint64(raw[0:8]),
		ConnectionID: binary.LittleEndian.Uint64(raw[8:16]),
		PID:          binary.LittleEndian.Uint32(raw[16:20]),
		TID:          binary.LittleEndian.Uint32(raw[20:24]),
		Length:       int32(binary.LittleEndian.Uint32(raw[24:28])),
		Direction:    raw[28],
		Function:     raw[29],
	}
	copy(event.Comm[:], raw[32:48])
	copy(event.Sample[:], raw[48:])

	if event.Length <= 0 {
		return bpfTSOpenSSLEvent{}, fmt.Errorf("bpf-ts OpenSSL event has non-positive length %d", event.Length)
	}
	if event.PID == 0 || event.TID == 0 {
		return bpfTSOpenSSLEvent{}, fmt.Errorf("bpf-ts OpenSSL event has invalid pid/tid %d/%d", event.PID, event.TID)
	}
	switch {
	case event.Direction == bpfTSOpenSSLDirectionSend && event.Function == tlsFuncSSLWrite:
	case event.Direction == bpfTSOpenSSLDirectionRecv && event.Function == tlsFuncSSLRead:
	default:
		return bpfTSOpenSSLEvent{}, fmt.Errorf(
			"bpf-ts OpenSSL event has invalid direction/function pair %d/%d",
			event.Direction, event.Function,
		)
	}
	return event, nil
}

func bpfTSOpenSSLToCompleted(event bpfTSOpenSSLEvent) CompletedTLSFragment {
	capturedLen := int(event.Length)
	flags := uint8(0)
	if capturedLen > bpfTSOpenSSLSampleSize {
		capturedLen = bpfTSOpenSSLSampleSize
		flags |= tlsFlagTruncated
	}

	payload := make([]byte, capturedLen)
	copy(payload, event.Sample[:capturedLen])
	return CompletedTLSFragment{
		TimestampNS:  event.TimestampNS,
		ConnectionID: event.ConnectionID,
		PID:          event.TID,
		TGID:         event.PID,
		DataLen:      uint32(capturedLen),
		TotalLen:     uint32(capturedLen),
		OriginalLen:  uint32(event.Length),
		FragCount:    1,
		LibType:      tlsLibOpenSSL,
		Direction:    event.Direction,
		Flags:        flags,
		Function:     event.Function,
		Comm:         sanitizeUTF8(event.Comm[:]),
		Payload:      payload,
	}
}
