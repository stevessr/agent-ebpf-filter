package tls

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2/hpack"
)

func testTLSHTTP2Frame(frameType, flags byte, streamID uint32, payload []byte) []byte {
	frame := make([]byte, tlsHTTP2FrameHeaderSize+len(payload))
	frame[0] = byte(len(payload) >> 16)
	frame[1] = byte(len(payload) >> 8)
	frame[2] = byte(len(payload))
	frame[3] = frameType
	frame[4] = flags
	binary.BigEndian.PutUint32(frame[5:9], streamID&0x7fffffff)
	copy(frame[tlsHTTP2FrameHeaderSize:], payload)
	return frame
}

func TestTLSHTTP2StreamAssemblerReassemblesSplitPreface(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	first := testCompletedTLSFragment(string(tlsHTTP2ClientPreface[:7]), tlsDirectionSend)
	first.ConnectionID = 0x1001
	second := testCompletedTLSFragment(string(tlsHTTP2ClientPreface[7:]), tlsDirectionSend)
	second.ConnectionID = first.ConnectionID
	second.TimestampNS = first.TimestampNS + uint64(time.Millisecond)

	if events, recognized := assembler.Add(first); !recognized || len(events) != 0 {
		t.Fatalf("first add = (%d events, recognized=%v), want (0, true)", len(events), recognized)
	}
	events, recognized := assembler.Add(second)
	if !recognized || len(events) != 1 {
		t.Fatalf("second add = (%d events, recognized=%v), want (1, true)", len(events), recognized)
	}
	if events[0].Type != "http2_preface" {
		t.Fatalf("Type = %q, want http2_preface", events[0].Type)
	}
	if assembler.Pending() != 0 {
		t.Fatalf("pending = %d, want 0", assembler.Pending())
	}
}

func TestTLSHTTP2StreamAssemblerReassemblesSplitDataFrame(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	frame := testTLSHTTP2Frame(0x0, 0x1, 1, []byte(`{"type":"message","text":"hello"}`))
	first := testCompletedTLSFragment(string(frame[:11]), tlsDirectionRecv)
	first.ConnectionID = 0x2002
	second := testCompletedTLSFragment(string(frame[11:]), tlsDirectionRecv)
	second.ConnectionID = first.ConnectionID
	second.TimestampNS = first.TimestampNS + uint64(time.Millisecond)

	if events, recognized := assembler.Add(first); !recognized || len(events) != 0 {
		t.Fatalf("first add = (%d events, recognized=%v), want (0, true)", len(events), recognized)
	}
	events, recognized := assembler.Add(second)
	if !recognized || len(events) != 1 {
		t.Fatalf("second add = (%d events, recognized=%v), want (1, true)", len(events), recognized)
	}
	if events[0].Type != "http2_data" {
		t.Fatalf("Type = %q, want http2_data", events[0].Type)
	}
	if events[0].Body != `{"type":"message","text":"hello"}` {
		t.Fatalf("Body = %q", events[0].Body)
	}
}

func TestTLSHTTP2StreamAssemblerRetainsKnownConnectionAcrossSplitHeader(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	connectionID := uint64(0x2020)
	preface := testCompletedTLSFragment(string(tlsHTTP2ClientPreface), tlsDirectionSend)
	preface.ConnectionID = connectionID
	if events, recognized := assembler.Add(preface); !recognized || len(events) != 1 {
		t.Fatalf("preface = (%d events, recognized=%v), want (1, true)", len(events), recognized)
	}

	frame := testTLSHTTP2Frame(0x0, 0x1, 3, []byte("next"))
	first := testCompletedTLSFragment(string(frame[:4]), tlsDirectionSend)
	first.ConnectionID = connectionID
	second := testCompletedTLSFragment(string(frame[4:]), tlsDirectionSend)
	second.ConnectionID = connectionID

	if events, recognized := assembler.Add(first); !recognized || len(events) != 0 {
		t.Fatalf("short header = (%d events, recognized=%v), want (0, true)", len(events), recognized)
	}
	events, recognized := assembler.Add(second)
	if !recognized || len(events) != 1 {
		t.Fatalf("header tail = (%d events, recognized=%v), want (1, true)", len(events), recognized)
	}
	if events[0].Type != "http2_data" || events[0].Body != "next" {
		t.Fatalf("unexpected event: %+v", events[0])
	}
}

func TestTLSHTTP2StreamAssemblerIsolatesConnections(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	one := testTLSHTTP2Frame(0x0, 0x1, 1, []byte("one"))
	two := testTLSHTTP2Frame(0x0, 0x1, 1, []byte("two"))

	firstOne := testCompletedTLSFragment(string(one[:10]), tlsDirectionRecv)
	firstOne.ConnectionID = 0xaaa
	firstTwo := testCompletedTLSFragment(string(two[:10]), tlsDirectionRecv)
	firstTwo.ConnectionID = 0xbbb

	if _, recognized := assembler.Add(firstOne); !recognized {
		t.Fatal("connection one was not recognized")
	}
	if _, recognized := assembler.Add(firstTwo); !recognized {
		t.Fatal("connection two was not recognized")
	}

	lastOne := testCompletedTLSFragment(string(one[10:]), tlsDirectionRecv)
	lastOne.ConnectionID = firstOne.ConnectionID
	lastTwo := testCompletedTLSFragment(string(two[10:]), tlsDirectionRecv)
	lastTwo.ConnectionID = firstTwo.ConnectionID

	eventsOne, _ := assembler.Add(lastOne)
	eventsTwo, _ := assembler.Add(lastTwo)
	if len(eventsOne) != 1 || eventsOne[0].Body != "one" {
		t.Fatalf("connection one events = %+v", eventsOne)
	}
	if len(eventsTwo) != 1 || eventsTwo[0].Body != "two" {
		t.Fatalf("connection two events = %+v", eventsTwo)
	}
}

func TestTLSHTTP2StreamAssemblerRecognizesSmallControlFrame(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	frame := testTLSHTTP2Frame(0x6, 0, 0, []byte("12345678"))
	fragment := testCompletedTLSFragment(string(frame), tlsDirectionRecv)
	fragment.ConnectionID = 0x333

	events, recognized := assembler.Add(fragment)
	if !recognized || len(events) != 1 {
		t.Fatalf("PING add = (%d events, recognized=%v), want (1, true)", len(events), recognized)
	}
	if events[0].Type != "http2_frame" {
		t.Fatalf("Type = %q, want http2_frame", events[0].Type)
	}
}

func TestTLSHTTP2StreamAssemblerRejectsInvalidHeader(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	fragment := testCompletedTLSFragment("plain text that is not http2", tlsDirectionRecv)
	if events, recognized := assembler.Add(fragment); recognized || len(events) != 0 {
		t.Fatalf("invalid add = (%d events, recognized=%v), want (0, false)", len(events), recognized)
	}
}

func TestTLSHTTP2StreamAssemblerDropsTruncatedRemainder(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	frame := testTLSHTTP2Frame(0x0, 0, 1, bytes.Repeat([]byte("x"), 64))
	fragment := testCompletedTLSFragment(string(frame[:20]), tlsDirectionRecv)
	fragment.ConnectionID = 0x444
	fragment.Flags = tlsFlagTruncated

	events, recognized := assembler.Add(fragment)
	if !recognized || len(events) != 1 {
		t.Fatalf("truncated add = (%d events, recognized=%v), want (1, true)", len(events), recognized)
	}
	if events[0].Type != "http2_capture_gap" || !events[0].Truncated {
		t.Fatalf("unexpected truncated gap: %+v", events[0])
	}
	if events[0].RawHexDump != "" {
		t.Fatalf("capture gap leaked raw payload: %q", events[0].RawHexDump)
	}
	if assembler.Pending() != 0 {
		t.Fatalf("pending = %d, want 0 after truncated payload", assembler.Pending())
	}
}

func TestTLSHTTP2StreamAssemblerEnrichesHPACKRequest(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	var encoded bytes.Buffer
	encoder := hpack.NewEncoder(&encoded)
	for _, field := range []hpack.HeaderField{
		{Name: ":method", Value: "POST"},
		{Name: ":path", Value: "/v1/messages?api_key=top-secret&safe=yes"},
		{Name: ":authority", Value: "api.example.com"},
		{Name: "content-type", Value: "application/json"},
		{Name: "authorization", Value: "Bearer should-not-leak"},
	} {
		if err := encoder.WriteField(field); err != nil {
			t.Fatalf("WriteField(%s): %v", field.Name, err)
		}
	}
	frame := testTLSHTTP2Frame(0x1, 0x4, 11, encoded.Bytes())
	fragment := testCompletedTLSFragment(string(frame), tlsDirectionSend)
	fragment.ConnectionID = 0x5050

	events, recognized := assembler.Add(fragment)
	if !recognized || len(events) != 1 {
		t.Fatalf("HEADERS add = (%d events, recognized=%v), want (1, true)", len(events), recognized)
	}
	event := events[0]
	if event.Type != "http2_request" || event.Method != "POST" || event.Host != "api.example.com" {
		t.Fatalf("unexpected HPACK request: %+v", event)
	}
	if strings.Contains(event.URL, "top-secret") || !strings.Contains(event.URL, "safe=yes") {
		t.Fatalf("URL was not sanitized: %q", event.URL)
	}
	if event.Headers["authorization"] != tlsRedactedValue {
		t.Fatalf("authorization = %q, want redacted", event.Headers["authorization"])
	}
	if event.RawHexDump != "" {
		t.Fatalf("HPACK request leaked compressed bytes: %q", event.RawHexDump)
	}
}

func TestTLSHTTP2StreamAssemblerKeepsCompleteFrameBeforeCaptureGap(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	frame := testTLSHTTP2Frame(0x0, 0, 13, []byte("complete"))
	payload := append(append([]byte(nil), frame...), []byte{0, 0, 8, 0}...)
	fragment := testCompletedTLSFragment(string(payload), tlsDirectionRecv)
	fragment.ConnectionID = 0x6060
	fragment.Flags = tlsFlagTruncated
	fragment.OriginalLen = uint32(len(payload) + 128)

	events, recognized := assembler.Add(fragment)
	if !recognized || len(events) != 2 {
		t.Fatalf("truncated complete+tail = (%d events, recognized=%v), want (2, true): %+v", len(events), recognized, events)
	}
	if events[0].Type != "http2_data" || events[0].Truncated {
		t.Fatalf("complete frame was incorrectly marked truncated: %+v", events[0])
	}
	if events[1].Type != "http2_capture_gap" || !events[1].Truncated {
		t.Fatalf("missing capture gap: %+v", events[1])
	}
	if events[0].RawHexDump != "" || events[1].RawHexDump != "" {
		t.Fatalf("HTTP2 output leaked raw data: frame=%q gap=%q", events[0].RawHexDump, events[1].RawHexDump)
	}
}
