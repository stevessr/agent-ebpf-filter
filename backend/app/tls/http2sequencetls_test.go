package tls

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2/hpack"
)

func testHTTP2EncodedHeaders(t *testing.T, fields ...hpack.HeaderField) []byte {
	t.Helper()
	var buffer bytes.Buffer
	encoder := hpack.NewEncoder(&buffer)
	for _, field := range fields {
		if err := encoder.WriteField(field); err != nil {
			t.Fatalf("WriteField(%s): %v", field.Name, err)
		}
	}
	return append([]byte(nil), buffer.Bytes()...)
}

func TestTLSHTTP2AssemblerAcceptsOrderedContinuation(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	block := testHTTP2EncodedHeaders(t,
		hpack.HeaderField{Name: ":method", Value: "GET"},
		hpack.HeaderField{Name: ":path", Value: "/v1/models"},
		hpack.HeaderField{Name: ":authority", Value: "api.example.com"},
		hpack.HeaderField{Name: "x-long", Value: strings.Repeat("value", 20)},
	)
	cut := len(block) / 2
	if cut == 0 {
		t.Fatal("encoded header block is empty")
	}

	firstFrame := testTLSHTTP2Frame(0x1, 0, 21, block[:cut])
	first := testCompletedTLSFragment(string(firstFrame), tlsDirectionSend)
	first.ConnectionID = 0x7001
	firstEvents, recognized := assembler.Add(first)
	if !recognized || len(firstEvents) != 1 {
		t.Fatalf("HEADERS = (%d events, recognized=%v), want (1, true)", len(firstEvents), recognized)
	}
	if firstEvents[0].Type != "http2_frame" {
		t.Fatalf("first type = %q, want http2_frame while waiting", firstEvents[0].Type)
	}

	secondFrame := testTLSHTTP2Frame(0x9, 0x4, 21, block[cut:])
	second := testCompletedTLSFragment(string(secondFrame), tlsDirectionSend)
	second.ConnectionID = first.ConnectionID
	secondEvents, recognized := assembler.Add(second)
	if !recognized || len(secondEvents) != 1 {
		t.Fatalf("CONTINUATION = (%d events, recognized=%v), want (1, true)", len(secondEvents), recognized)
	}
	event := secondEvents[0]
	if event.Type != "http2_request" || event.Method != "GET" || event.URL != "/v1/models" {
		t.Fatalf("ordered continuation was not decoded: %+v", event)
	}
}

func TestTLSHTTP2AssemblerRejectsInterleavedFrameDuringHeaders(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	block := testHTTP2EncodedHeaders(t,
		hpack.HeaderField{Name: ":method", Value: "POST"},
		hpack.HeaderField{Name: ":path", Value: "/v1/messages"},
		hpack.HeaderField{Name: ":authority", Value: "api.example.com"},
	)
	cut := len(block) / 2
	if cut == 0 {
		t.Fatal("encoded header block is empty")
	}

	firstFrame := testTLSHTTP2Frame(0x1, 0, 23, block[:cut])
	first := testCompletedTLSFragment(string(firstFrame), tlsDirectionSend)
	first.ConnectionID = 0x7002
	if events, recognized := assembler.Add(first); !recognized || len(events) != 1 {
		t.Fatalf("HEADERS = (%d events, recognized=%v), want (1, true)", len(events), recognized)
	}

	secret := `{"api_key":"must-not-leak","text":"payload"}`
	dataFrame := testTLSHTTP2Frame(0x0, 0, 23, []byte(secret))
	data := testCompletedTLSFragment(string(dataFrame), tlsDirectionSend)
	data.ConnectionID = first.ConnectionID
	events, recognized := assembler.Add(data)
	if !recognized || len(events) != 1 {
		t.Fatalf("interleaved DATA = (%d events, recognized=%v), want (1, true)", len(events), recognized)
	}
	event := events[0]
	if event.Type != "http2_headers_sequence_error" || event.DataType != "protocol_error" {
		t.Fatalf("unexpected sequence event: %+v", event)
	}
	if event.Body != "" || event.RawHexDump != "" || event.RawAvailable {
		t.Fatalf("sequence error leaked payload: %+v", event)
	}
}

func TestTLSHTTP2AssemblerRejectsOrphanContinuation(t *testing.T) {
	assembler := NewTLSHTTP2StreamAssembler(10 * time.Second)
	frame := testTLSHTTP2Frame(0x9, 0x4, 25, []byte{0x82, 0x84})
	fragment := testCompletedTLSFragment(string(frame), tlsDirectionRecv)
	fragment.ConnectionID = 0x7003

	events, recognized := assembler.Add(fragment)
	if !recognized || len(events) != 1 {
		t.Fatalf("orphan CONTINUATION = (%d events, recognized=%v), want (1, true)", len(events), recognized)
	}
	if events[0].Type != "http2_headers_sequence_error" {
		t.Fatalf("type = %q, want sequence error", events[0].Type)
	}
}
