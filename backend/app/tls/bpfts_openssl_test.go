package tls

import (
	"encoding/binary"
	"testing"
)

func encodeBpfTSOpenSSLFixture(t *testing.T, length int32, direction, function uint8) []byte {
	t.Helper()
	raw := make([]byte, bpfTSOpenSSLEventSize)
	binary.LittleEndian.PutUint64(raw[0:8], 123456789)
	binary.LittleEndian.PutUint64(raw[8:16], 0xfeedbeef)
	binary.LittleEndian.PutUint32(raw[16:20], 4242)
	binary.LittleEndian.PutUint32(raw[20:24], 4243)
	binary.LittleEndian.PutUint32(raw[24:28], uint32(length))
	raw[28] = direction
	raw[29] = function
	copy(raw[32:48], []byte("agent-test"))
	for i := 0; i < bpfTSOpenSSLSampleSize; i++ {
		raw[48+i] = byte(i % 251)
	}
	return raw
}

func TestBpfTSOpenSSLDecodeWriteEvent(t *testing.T) {
	raw := encodeBpfTSOpenSSLFixture(t, 128, bpfTSOpenSSLDirectionSend, tlsFuncSSLWrite)
	event, err := decodeBpfTSOpenSSLEvent(raw)
	if err != nil {
		t.Fatalf("decodeBpfTSOpenSSLEvent() error = %v", err)
	}
	completed := bpfTSOpenSSLToCompleted(event)
	if completed.PID != 4243 || completed.TGID != 4242 {
		t.Fatalf("PID/TGID = %d/%d, want 4243/4242", completed.PID, completed.TGID)
	}
	if completed.ConnectionID != 0xfeedbeef {
		t.Fatalf("ConnectionID = %#x", completed.ConnectionID)
	}
	if completed.Direction != tlsDirectionSend || completed.Function != tlsFuncSSLWrite {
		t.Fatalf("direction/function = %d/%d", completed.Direction, completed.Function)
	}
	if completed.OriginalLen != 128 || completed.TotalLen != 128 || len(completed.Payload) != 128 {
		t.Fatalf("lengths = original:%d total:%d payload:%d", completed.OriginalLen, completed.TotalLen, len(completed.Payload))
	}
	if completed.Truncated() {
		t.Fatal("128-byte event unexpectedly truncated")
	}
	if completed.Comm != "agent-test" {
		t.Fatalf("Comm = %q", completed.Comm)
	}
}

func TestBpfTSOpenSSLDecodeReadEventAndTruncation(t *testing.T) {
	raw := encodeBpfTSOpenSSLFixture(t, 9000, bpfTSOpenSSLDirectionRecv, tlsFuncSSLRead)
	event, err := decodeBpfTSOpenSSLEvent(raw)
	if err != nil {
		t.Fatalf("decodeBpfTSOpenSSLEvent() error = %v", err)
	}
	completed := bpfTSOpenSSLToCompleted(event)
	if completed.OriginalLen != 9000 || completed.TotalLen != bpfTSOpenSSLSampleSize || len(completed.Payload) != bpfTSOpenSSLSampleSize {
		t.Fatalf("truncated lengths = original:%d total:%d payload:%d", completed.OriginalLen, completed.TotalLen, len(completed.Payload))
	}
	if completed.Flags&tlsFlagTruncated == 0 {
		t.Fatal("9000-byte event did not set tlsFlagTruncated")
	}
}

func TestBpfTSOpenSSLDecodeRejectsABIDrift(t *testing.T) {
	if _, err := decodeBpfTSOpenSSLEvent(make([]byte, bpfTSOpenSSLEventSize-1)); err == nil {
		t.Fatal("expected short ABI record to fail")
	}
	raw := encodeBpfTSOpenSSLFixture(t, 64, bpfTSOpenSSLDirectionSend, tlsFuncSSLWrite)
	binary.LittleEndian.PutUint16(raw[30:32], 1)
	if _, err := decodeBpfTSOpenSSLEvent(raw); err == nil {
		t.Fatal("expected non-zero reserved ABI field to fail")
	}
}

func TestBpfTSOpenSSLDecodeRejectsInvalidDirectionFunctionPair(t *testing.T) {
	raw := encodeBpfTSOpenSSLFixture(t, 64, bpfTSOpenSSLDirectionSend, tlsFuncSSLRead)
	if _, err := decodeBpfTSOpenSSLEvent(raw); err == nil {
		t.Fatal("expected send/SSL_read mismatch to fail")
	}
}

func (fragment CompletedTLSFragment) Truncated() bool {
	return fragment.Flags&tlsFlagTruncated != 0
}
