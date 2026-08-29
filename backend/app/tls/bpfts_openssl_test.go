package tls

import (
	"encoding/binary"
	"testing"
)

func encodeBpfTSOpenSSLFixtureWithWirePayload(t *testing.T, length int32, direction, function uint8, wirePayload int) []byte {
	t.Helper()
	raw := make([]byte, bpfTSOpenSSLMetadataSize+wirePayload)
	binary.LittleEndian.PutUint64(raw[0:8], 123456789)
	binary.LittleEndian.PutUint64(raw[8:16], 0xfeedbeef)
	binary.LittleEndian.PutUint32(raw[16:20], 4242)
	binary.LittleEndian.PutUint32(raw[20:24], 4243)
	binary.LittleEndian.PutUint32(raw[24:28], uint32(length))
	raw[28] = direction
	raw[29] = function
	copy(raw[32:48], []byte("agent-test"))
	for i := 0; i < wirePayload; i++ {
		raw[48+i] = byte(i % 251)
	}
	return raw
}

func encodeBpfTSOpenSSLFixture(t *testing.T, length int32, direction, function uint8) []byte {
	t.Helper()
	return encodeBpfTSOpenSSLFixtureWithWirePayload(
		t, length, direction, function, bpfTSOpenSSLSampleSize,
	)
}

func encodeCompactBpfTSOpenSSLFixture(t *testing.T, length int32, direction, function uint8) []byte {
	t.Helper()
	return encodeBpfTSOpenSSLFixtureWithWirePayload(
		t, length, direction, function, expectedBpfTSOpenSSLCapturedLen(length),
	)
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

func TestBpfTSOpenSSLDecodeCompactWriteEvent(t *testing.T) {
	raw := encodeCompactBpfTSOpenSSLFixture(t, 128, bpfTSOpenSSLDirectionSend, tlsFuncSSLWrite)
	if len(raw) != bpfTSOpenSSLMetadataSize+128 {
		t.Fatalf("compact wire len = %d", len(raw))
	}
	event, err := decodeBpfTSOpenSSLEvent(raw)
	if err != nil {
		t.Fatalf("decode compact event error = %v", err)
	}
	if event.CapturedLen != 128 {
		t.Fatalf("CapturedLen = %d, want 128", event.CapturedLen)
	}
	completed := bpfTSOpenSSLToCompleted(event)
	if completed.TotalLen != 128 || len(completed.Payload) != 128 || completed.Truncated() {
		t.Fatalf("unexpected compact completed fragment: %+v", completed)
	}
}

func TestBpfTSOpenSSLDecodeReadEventAndTruncation(t *testing.T) {
	raw := encodeCompactBpfTSOpenSSLFixture(t, 9000, bpfTSOpenSSLDirectionRecv, tlsFuncSSLRead)
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
	if _, err := decodeBpfTSOpenSSLEvent(make([]byte, bpfTSOpenSSLMetadataSize-1)); err == nil {
		t.Fatal("expected undersized metadata record to fail")
	}
	raw := encodeBpfTSOpenSSLFixture(t, 64, bpfTSOpenSSLDirectionSend, tlsFuncSSLWrite)
	binary.LittleEndian.PutUint16(raw[30:32], 1)
	if _, err := decodeBpfTSOpenSSLEvent(raw); err == nil {
		t.Fatal("expected non-zero reserved ABI field to fail")
	}

	malformedCompact := encodeBpfTSOpenSSLFixtureWithWirePayload(
		t, 64, bpfTSOpenSSLDirectionSend, tlsFuncSSLWrite, 63,
	)
	if _, err := decodeBpfTSOpenSSLEvent(malformedCompact); err == nil {
		t.Fatal("expected compact payload length mismatch to fail")
	}
}

func TestBpfTSOpenSSLDecodeRejectsInvalidDirectionFunctionPair(t *testing.T) {
	raw := encodeCompactBpfTSOpenSSLFixture(t, 64, bpfTSOpenSSLDirectionSend, tlsFuncSSLRead)
	if _, err := decodeBpfTSOpenSSLEvent(raw); err == nil {
		t.Fatal("expected send/SSL_read mismatch to fail")
	}
}

func (fragment CompletedTLSFragment) Truncated() bool {
	return fragment.Flags&tlsFlagTruncated != 0
}
