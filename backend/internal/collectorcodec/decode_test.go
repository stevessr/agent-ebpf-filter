package collectorcodec

import (
	"encoding/binary"
	"errors"
	"testing"
	"unsafe"
)

type record struct {
	PID uint32
	Kind uint32
}

func alignedSample(size int, alignment uintptr) []byte {
	raw := make([]byte, size+int(alignment))
	base := uintptr(unsafe.Pointer(&raw[0]))
	offset := int((alignment-base%alignment)%alignment)
	return raw[offset : offset+size]
}

func TestDecodeAlignedCanUseZeroCopy(t *testing.T) {
	sample := alignedSample(8, unsafe.Alignof(record{}))
	binary.LittleEndian.PutUint32(sample[0:4], 1234)
	binary.LittleEndian.PutUint32(sample[4:8], 9)

	event, zeroCopy, err := Decode[record](sample)
	if err != nil {
		t.Fatal(err)
	}
	if event.PID != 1234 || event.Kind != 9 {
		t.Fatalf("decoded = %+v", event)
	}
	if NativeLittleEndian != zeroCopy {
		t.Fatalf("zeroCopy=%v on nativeLittleEndian=%v", zeroCopy, NativeLittleEndian)
	}
	if zeroCopy {
		event.PID = 777
		if got := binary.LittleEndian.Uint32(sample[:4]); got != 777 {
			t.Fatalf("zero-copy record did not alias original sample: %d", got)
		}
	}
}

func TestDecodeMisalignedMakesIndependentCopy(t *testing.T) {
	const recordSize = 8
	raw := make([]byte, recordSize+8)
	var sample []byte
	for offset := 0; offset < 8; offset++ {
		candidate := raw[offset : offset+recordSize]
		if uintptr(unsafe.Pointer(&candidate[0]))%unsafe.Alignof(record{}) != 0 {
			sample = candidate
			break
		}
	}
	if sample == nil {
		t.Skip("unable to obtain a misaligned sample on this target")
	}
	binary.LittleEndian.PutUint32(sample[0:4], 200)
	binary.LittleEndian.PutUint32(sample[4:8], 10)
	event, zeroCopy, err := Decode[record](sample)
	if err != nil {
		t.Fatal(err)
	}
	if zeroCopy {
		t.Fatal("misaligned sample was aliased")
	}
	sample[0] = 0
	if event.PID != 200 || event.Kind != 10 {
		t.Fatalf("copied record mutated with sample: %+v", event)
	}
}

func TestDecodeRejectsShortAndEmptyLayout(t *testing.T) {
	if _, _, err := Decode[record](make([]byte, 7)); err == nil {
		t.Fatal("short sample accepted")
	}
	if _, _, err := Decode[struct{}](nil); !errors.Is(err, ErrEmptyRecord) {
		t.Fatalf("empty layout error = %v", err)
	}
}
