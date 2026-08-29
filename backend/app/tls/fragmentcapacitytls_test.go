package tls

import (
	"testing"
	"time"
)

func TestFragmentAssemblerReassemblesExpandedCaptureWindow(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	const timestamp = uint64(123456789)
	const connectionID = uint64(0x35_712)
	totalLen := uint32(tlsFragmentSize * tlsMaxFragments)

	var completed *CompletedTLSFragment
	for index := 0; index < tlsMaxFragments; index++ {
		fragment := tlsFragment{
			TimestampNS:  timestamp,
			ConnectionID: connectionID,
			PID:          1234,
			TGID:         1234,
			DataLen:      uint32(tlsFragmentSize),
			TotalLen:     totalLen,
			OriginalLen:  totalLen,
			FragIndex:    uint16(index),
			FragCount:    uint16(tlsMaxFragments),
			LibType:      tlsLibOpenSSL,
			Direction:    tlsDirectionRecv,
			Function:     tlsFuncSSLRead,
		}
		for i := range fragment.Data {
			fragment.Data[i] = byte((index + i) & 0xff)
		}

		var ok bool
		completed, ok = assembler.Add(fragment)
		if index < tlsMaxFragments-1 {
			if ok || completed != nil {
				t.Fatalf("fragment %d completed early: ok=%v completed=%v", index, ok, completed != nil)
			}
			continue
		}
		if !ok || completed == nil {
			t.Fatalf("final fragment did not complete: ok=%v completed=%v", ok, completed != nil)
		}
	}

	if got, want := len(completed.Payload), int(totalLen); got != want {
		t.Fatalf("reassembled payload len=%d want=%d", got, want)
	}
	if totalLen != 35712 {
		t.Fatalf("capture window=%d, want 35712", totalLen)
	}
	if completed.Payload[0] != 0 || completed.Payload[tlsFragmentSize] != 1 {
		t.Fatalf("fragment boundary data mismatch: first=%d second=%d", completed.Payload[0], completed.Payload[tlsFragmentSize])
	}
	if assembler.Pending() != 0 {
		t.Fatalf("pending=%d after completion, want 0", assembler.Pending())
	}
}

func TestFragmentAssemblerRejectsFragmentCountBeyondCaptureWindow(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	fragment := tlsFragment{
		TimestampNS:  123456789,
		ConnectionID: 0xdead,
		PID:          1234,
		TGID:         1234,
		DataLen:      1,
		TotalLen:     uint32(tlsFragmentSize*tlsMaxFragments + 1),
		OriginalLen:  uint32(tlsFragmentSize*tlsMaxFragments + 1),
		FragIndex:    0,
		FragCount:    uint16(tlsMaxFragments + 1),
		LibType:      tlsLibOpenSSL,
		Direction:    tlsDirectionRecv,
		Function:     tlsFuncSSLRead,
	}
	fragment.Data[0] = 'x'

	if completed, ok := assembler.Add(fragment); ok || completed != nil {
		t.Fatalf("oversized fragment count accepted: ok=%v completed=%v", ok, completed != nil)
	}
	if assembler.Dropped() != 1 {
		t.Fatalf("dropped=%d want=1", assembler.Dropped())
	}
}
