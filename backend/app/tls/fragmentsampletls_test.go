package tls

import (
	"encoding/binary"
	"testing"
)

func encodeTLSFragmentSampleForTest(t *testing.T, fragment tlsFragment, compact bool) []byte {
	t.Helper()
	wireLen := tlsFragmentMetadataSize + int(fragment.DataLen)
	if !compact {
		// Match sizeof(struct tls_fragment): metadata + 960 data bytes + 4 bytes
		// of tail padding for 8-byte struct alignment.
		wireLen = tlsFragmentMetadataSize + tlsFragmentSize + 4
	}
	raw := make([]byte, wireLen)
	binary.LittleEndian.PutUint64(raw[0:8], fragment.TimestampNS)
	binary.LittleEndian.PutUint64(raw[8:16], fragment.ConnectionID)
	binary.LittleEndian.PutUint32(raw[16:20], fragment.PID)
	binary.LittleEndian.PutUint32(raw[20:24], fragment.TGID)
	binary.LittleEndian.PutUint32(raw[24:28], fragment.DataLen)
	binary.LittleEndian.PutUint32(raw[28:32], fragment.TotalLen)
	binary.LittleEndian.PutUint32(raw[32:36], fragment.OriginalLen)
	binary.LittleEndian.PutUint16(raw[36:38], fragment.FragIndex)
	binary.LittleEndian.PutUint16(raw[38:40], fragment.FragCount)
	raw[40] = fragment.LibType
	raw[41] = fragment.Direction
	raw[42] = fragment.Flags
	raw[43] = fragment.Function
	copy(raw[44:60], fragment.Comm[:])
	copy(raw[60:], fragment.Data[:fragment.DataLen])
	return raw
}

func TestDecodeTLSFragmentSampleCompact(t *testing.T) {
	fragment := newTestTLSFragmentAt(1, 3, 12, "hello", 123456789)
	fragment.ConnectionID = 0x1234
	fragment.Function = tlsFuncSSLReadEx
	fragment.Flags = tlsFlagTruncated

	decoded, err := decodeTLSFragmentSample(encodeTLSFragmentSampleForTest(t, fragment, true))
	if err != nil {
		t.Fatalf("decode compact sample: %v", err)
	}
	if decoded.TimestampNS != fragment.TimestampNS || decoded.ConnectionID != fragment.ConnectionID {
		t.Fatalf("identity mismatch: %+v", decoded)
	}
	if decoded.DataLen != 5 || string(decoded.Data[:decoded.DataLen]) != "hello" {
		t.Fatalf("payload mismatch: len=%d payload=%q", decoded.DataLen, decoded.Data[:decoded.DataLen])
	}
	if decoded.Function != tlsFuncSSLReadEx || decoded.Flags != tlsFlagTruncated {
		t.Fatalf("metadata mismatch: function=%d flags=%d", decoded.Function, decoded.Flags)
	}
}

func TestDecodeTLSFragmentSampleLegacyFixedSize(t *testing.T) {
	fragment := newTestTLSFragment(0, 1, 3, "abc")
	raw := encodeTLSFragmentSampleForTest(t, fragment, false)
	if len(raw) != 1024 {
		t.Fatalf("legacy sample size = %d, want 1024", len(raw))
	}

	decoded, err := decodeTLSFragmentSample(raw)
	if err != nil {
		t.Fatalf("decode legacy sample: %v", err)
	}
	if got := string(decoded.Data[:decoded.DataLen]); got != "abc" {
		t.Fatalf("payload = %q, want abc", got)
	}
}

func TestDecodeTLSFragmentSampleRejectsTruncatedWirePayload(t *testing.T) {
	fragment := newTestTLSFragment(0, 1, 5, "abcde")
	raw := encodeTLSFragmentSampleForTest(t, fragment, true)
	raw = raw[:len(raw)-1]
	if _, err := decodeTLSFragmentSample(raw); err == nil {
		t.Fatal("expected truncated compact sample to fail")
	}
}

func TestDecodeTLSFragmentSampleRejectsInvalidDataLength(t *testing.T) {
	raw := make([]byte, tlsFragmentMetadataSize)
	binary.LittleEndian.PutUint32(raw[24:28], tlsFragmentSize+1)
	if _, err := decodeTLSFragmentSample(raw); err == nil {
		t.Fatal("expected oversized data_len to fail")
	}
}

func TestTLSFragmentMetadataSizeMatchesDataOffset(t *testing.T) {
	// This is intentionally a wire-layout assertion rather than unsafe.Offsetof:
	// binary perf records are decoded field-by-field and must remain stable even
	// if Go's in-memory struct padding ever changes.
	fragment := newTestTLSFragment(0, 1, 1, "x")
	raw := encodeTLSFragmentSampleForTest(t, fragment, true)
	if len(raw) != tlsFragmentMetadataSize+1 || raw[tlsFragmentMetadataSize] != 'x' {
		t.Fatalf("unexpected compact wire layout: len=%d payload=%q", len(raw), raw[tlsFragmentMetadataSize:])
	}
}
