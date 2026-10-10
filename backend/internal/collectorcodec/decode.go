// Package collectorcodec decodes a fixed-layout eBPF ring-buffer sample into a
// typed Go record, without importing app or depending on generated BPF types.
//
// A zero-copy return aliases raw. The caller MUST finish using the result
// before the next ReadInto overwrites that sample buffer. The fallback decoder
// uses the existing little-endian binary.Read behavior for unaligned samples.
package collectorcodec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"
)

var ErrEmptyRecord = errors.New("empty eBPF record layout")

// NativeLittleEndian reports whether host byte order matches the generated
// little-endian BPF objects currently used by the collector.
var NativeLittleEndian = func() bool {
	var value uint16 = 1
	return *(*byte)(unsafe.Pointer(&value)) == 1
}()

// Decode returns a zero-copy pointer when sample length, endian and alignment
// permit; otherwise it reads a detached copy. T must be a fixed binary layout
// compatible with binary.Read for the fallback path.
func Decode[T any](raw []byte) (*T, bool, error) {
	var event T
	size := int(unsafe.Sizeof(event))
	if size == 0 {
		return nil, false, ErrEmptyRecord
	}
	if len(raw) < size {
		return nil, false, fmt.Errorf("short eBPF event sample: got %d bytes, want at least %d", len(raw), size)
	}
	if NativeLittleEndian {
		ptr := unsafe.Pointer(&raw[0])
		if uintptr(ptr)%unsafe.Alignof(event) == 0 {
			return (*T)(ptr), true, nil
		}
	}
	if err := binary.Read(bytes.NewReader(raw[:size]), binary.LittleEndian, &event); err != nil {
		return nil, false, err
	}
	return &event, false, nil
}
