package main

import (
	"bytes"
	"fmt"
	"testing"
	"time"
	"unsafe"

	bpf "agent-ebpf-filter/ebpf"
)

func newTestTLSFragment(index, count int, totalLen int, data string) tlsFragment {
	return newTestTLSFragmentAt(index, count, totalLen, data, uint64(time.Now().UnixNano()))
}

func newTestTLSFragmentAt(index, count int, totalLen int, data string, timestampNS uint64) tlsFragment {
	var frag tlsFragment
	frag.TimestampNS = timestampNS
	frag.PID = 1234
	frag.TGID = 5678
	frag.DataLen = uint32(len(data))
	frag.TotalLen = uint32(totalLen)
	frag.FragIndex = uint16(index)
	frag.FragCount = uint16(count)
	frag.LibType = tlsLibOpenSSL
	frag.Direction = tlsDirectionSend
	copy(frag.Comm[:], []byte("curl"))
	copy(frag.Data[:], []byte(data))
	return frag
}

func TestFragmentAssemblerReassemblesOutOfOrderFragments(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)

	timestamp := uint64(time.Now().UnixNano())
	frags := []tlsFragment{
		newTestTLSFragmentAt(1, 3, 12, "fghij", timestamp),
		newTestTLSFragmentAt(0, 3, 12, "abcde", timestamp),
		newTestTLSFragmentAt(2, 3, 12, "kl", timestamp),
	}

	var completed *completedTLSFragment
	var ok bool
	for _, frag := range frags {
		completed, ok = assembler.Add(frag)
	}

	if !ok {
		t.Fatalf("expected completed fragment")
	}
	if completed == nil {
		t.Fatalf("expected completed fragment payload")
	}
	if got := string(completed.Payload); got != "abcdefghijkl" {
		t.Fatalf("unexpected payload: %q", got)
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("expected no pending fragments, got %d", got)
	}
}

func TestFragmentAssemblerCleansExpiredPendingBuffers(t *testing.T) {
	assembler := NewFragmentAssembler(time.Millisecond)
	frag := newTestTLSFragment(0, 2, 10, "abcde")
	frag.TimestampNS = uint64(time.Now().Add(-time.Second).UnixNano())

	if completed, ok := assembler.Add(frag); ok || completed != nil {
		t.Fatalf("expected incomplete fragment to stay pending")
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("expected one pending fragment, got %d", got)
	}
	if cleaned := assembler.CleanupExpired(time.Now()); cleaned != 1 {
		t.Fatalf("expected one expired fragment cleaned, got %d", cleaned)
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("expected no pending fragments after cleanup, got %d", got)
	}
}

func TestFragmentAssemblerRejectsInvalidFragments(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	frag := newTestTLSFragment(0, 0, 10, "abcde")

	if completed, ok := assembler.Add(frag); ok || completed != nil {
		t.Fatalf("expected invalid fragment to be rejected")
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("expected no pending fragments, got %d", got)
	}
	if got := assembler.Dropped(); got != 1 {
		t.Fatalf("expected one dropped fragment, got %d", got)
	}
}

func TestFragmentAssemblerDropsDuplicateFragmentIndexWithoutOverwriting(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())

	frag0 := newTestTLSFragmentAt(0, 2, 10, "abcde", timestamp)
	frag1 := newTestTLSFragmentAt(0, 2, 10, "vwxyz", timestamp)
	frag2 := newTestTLSFragmentAt(1, 2, 10, "fghij", timestamp)

	if completed, ok := assembler.Add(frag0); ok || completed != nil {
		t.Fatalf("expected first fragment to remain pending")
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("expected one pending fragment, got %d", got)
	}

	if completed, ok := assembler.Add(frag1); ok || completed != nil {
		t.Fatalf("expected duplicate fragment index to be rejected")
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("expected one pending fragment after duplicate, got %d", got)
	}
	if got := assembler.Dropped(); got != 1 {
		t.Fatalf("expected one dropped fragment after duplicate, got %d", got)
	}

	if completed, ok := assembler.Add(frag2); !ok || completed == nil {
		t.Fatalf("expected completed fragment after second unique fragment")
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("expected no pending fragments after completion, got %d", got)
	}
}

func TestFragmentAssemblerDistinguishesPendingEntriesByPID(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())

	fragA := newTestTLSFragmentAt(0, 2, 10, "abcde", timestamp)
	fragA.PID = 1111
	fragA.TGID = 5678
	fragB := newTestTLSFragmentAt(0, 2, 10, "vwxyz", timestamp)
	fragB.PID = 2222
	fragB.TGID = 5678

	if completed, ok := assembler.Add(fragA); ok || completed != nil {
		t.Fatalf("expected first fragment to remain pending")
	}
	if completed, ok := assembler.Add(fragB); ok || completed != nil {
		t.Fatalf("expected second fragment with different PID to remain pending")
	}
	if got := assembler.Pending(); got != 2 {
		t.Fatalf("expected two pending fragments for distinct PIDs, got %d", got)
	}
}

func TestFragmentAssemblerDropsOldestPendingWhenCapExceeded(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	base := uint64(time.Now().UnixNano())

	for i := 0; i < tlsMaxPendingFragments+1; i++ {
		frag := newTestTLSFragmentAt(0, 2, 10, "abcde", base+uint64(i))
		frag.PID = uint32(1000 + i)
		frag.TGID = 5678
		if completed, ok := assembler.Add(frag); ok || completed != nil {
			t.Fatalf("expected fragment %d to remain pending", i)
		}
	}

	if got := assembler.Pending(); got != tlsMaxPendingFragments {
		t.Fatalf("expected pending fragments to be capped at %d, got %d", tlsMaxPendingFragments, got)
	}
	if got := assembler.Dropped(); got != 1 {
		t.Fatalf("expected one dropped fragment from cap enforcement, got %d", got)
	}

	first := newTestTLSFragmentAt(1, 2, 10, "fghij", base)
	first.PID = 1000
	first.TGID = 5678
	if completed, ok := assembler.Add(first); ok || completed != nil {
		t.Fatalf("expected evicted oldest fragment key to start a new pending assembly")
	}
	if got := assembler.Pending(); got != tlsMaxPendingFragments {
		t.Fatalf("expected pending fragments to stay capped at %d after reinserting evicted key, got %d", tlsMaxPendingFragments, got)
	}
}

func TestFragmentAssemblerDropsPendingWhenCountOrLengthMismatchAppearsForSameKey(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())

	frag0 := newTestTLSFragment(0, 2, 10, "abcde")
	frag0.TimestampNS = timestamp
	frag1 := newTestTLSFragment(1, 3, 12, "fghij")
	frag1.TimestampNS = timestamp

	if completed, ok := assembler.Add(frag0); ok || completed != nil {
		t.Fatalf("expected first fragment to remain pending")
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("expected one pending fragment, got %d", got)
	}

	if completed, ok := assembler.Add(frag1); ok || completed != nil {
		t.Fatalf("expected mismatched fragment to be rejected")
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("expected pending fragment to be deleted after mismatch, got %d", got)
	}
	if got := assembler.Dropped(); got != 1 {
		t.Fatalf("expected one dropped fragment after mismatch, got %d", got)
	}
}

func TestTLSFragmentLayoutMatchesGeneratedBPFStruct(t *testing.T) {
	if got, want := unsafe.Sizeof(tlsFragment{}), unsafe.Sizeof(bpf.AgentTlsCaptureTlsFragment{}); got != want {
		t.Fatalf("unexpected tlsFragment size: got %d want %d", got, want)
	}
}

func TestCompletedFragmentPayloadIsCopied(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())
	frag0 := newTestTLSFragmentAt(0, 2, 6, "abc", timestamp)
	frag0.DataLen = 3
	frag0.TotalLen = 6
	frag1 := newTestTLSFragmentAt(1, 2, 6, "def", timestamp)
	frag1.DataLen = 3
	frag1.TotalLen = 6

	if completed, ok := assembler.Add(frag0); ok || completed != nil {
		t.Fatalf("expected first fragment to remain pending")
	}
	completed, ok := assembler.Add(frag1)
	if !ok || completed == nil {
		t.Fatalf("expected completed fragment")
	}
	if got := string(completed.Payload); got != "abcdef" {
		t.Fatalf("unexpected payload: %q", got)
	}
	orig := append([]byte(nil), completed.Payload...)
	frag0.Data[0] = 'z'
	frag1.Data[0] = 'y'
	if !bytes.Equal(completed.Payload, orig) {
		t.Fatalf("payload changed after source fragment mutation: %q", string(completed.Payload))
	}
}

func TestFragmentAssemblerRemoveByTGIDCleansPendingFragments(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())
	fragA := newTestTLSFragmentAt(0, 2, 10, "aaaaa", timestamp)
	fragA.TGID = 100
	fragB := newTestTLSFragmentAt(0, 2, 10, "bbbbb", timestamp+1)
	fragB.TGID = 200
	if _, ok := assembler.Add(fragA); ok {
		t.Fatalf("expected fragA to remain pending")
	}
	if _, ok := assembler.Add(fragB); ok {
		t.Fatalf("expected fragB to remain pending")
	}
	if got := assembler.Pending(); got != 2 {
		t.Fatalf("pending = %d, want 2", got)
	}
	removed := assembler.RemoveByTGID(100)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("pending after remove = %d, want 1", got)
	}
	removed = assembler.RemoveByTGID(999)
	if removed != 0 {
		t.Fatalf("removed nonexistent = %d, want 0", removed)
	}
}

func TestTLSPipelineAssembleAndParseHTTPRequestIntegration(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())

	body := `{"model":"claude-opus-4-7","messages":["hello"]}`
	httpPayload := fmt.Sprintf("POST /v1/messages HTTP/1.1\r\nHost: api.anthropic.com\r\nAuthorization: Bearer sk-ant-secret\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	totalLen := uint32(len(httpPayload))
	split := len(httpPayload) / 2

	frag0 := newTestTLSFragmentAt(0, 2, int(totalLen), httpPayload[:split], timestamp)
	frag0.LibType = tlsLibGo
	frag0.DataLen = uint32(split)
	frag1 := newTestTLSFragmentAt(1, 2, int(totalLen), httpPayload[split:], timestamp)
	frag1.LibType = tlsLibGo
	frag1.DataLen = uint32(len(httpPayload) - split)

	assembler.Add(frag0)
	completed, ok := assembler.Add(frag1)
	if !ok || completed == nil {
		t.Fatalf("expected completed fragment from pipeline")
	}

	event := parseTLSPlaintext(*completed)
	if event.Type != "http_request" && event.Type != "tls_plaintext" {
		t.Fatalf("unexpected event type: %q", event.Type)
	}
	if event.Method != "POST" {
		t.Fatalf("method = %q, want POST", event.Method)
	}
	if event.URL != "/v1/messages" {
		t.Fatalf("url = %q, want /v1/messages", event.URL)
	}
	if event.Host != "api.anthropic.com" {
		t.Fatalf("host = %q, want api.anthropic.com", event.Host)
	}
	if event.Direction != "send" {
		t.Fatalf("direction = %q, want send", event.Direction)
	}
	if event.Lib != "go" {
		t.Fatalf("lib = %q, want go", event.Lib)
	}
	if event.Headers["authorization"] != "***REDACTED***" {
		t.Fatalf("authorization not redacted: %q", event.Headers["authorization"])
	}
	if event.ContentType != "application/json" {
		t.Fatalf("content_type = %q, want application/json", event.ContentType)
	}
	if event.BodySize <= 0 {
		t.Fatalf("body_size = %d, want >0", event.BodySize)
	}
}

func TestFragmentAssemblerRemoveByPIDCleansPendingFragments(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())
	fragA := newTestTLSFragmentAt(0, 2, 10, "aaaaa", timestamp)
	fragA.PID = 42
	fragB := newTestTLSFragmentAt(0, 2, 10, "bbbbb", timestamp+1)
	fragB.PID = 43
	if _, ok := assembler.Add(fragA); ok {
		t.Fatalf("expected fragA to remain pending")
	}
	if _, ok := assembler.Add(fragB); ok {
		t.Fatalf("expected fragB to remain pending")
	}
	if got := assembler.Pending(); got != 2 {
		t.Fatalf("pending = %d, want 2", got)
	}
	removed := assembler.RemoveByPID(42)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("pending after remove = %d, want 1", got)
	}
}
