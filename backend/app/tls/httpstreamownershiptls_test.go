package tls

import (
	"strings"
	"testing"
	"time"
)

func TestTLSHTTPStreamAssemblerOwnsSplitFirstLinePrefix(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	first := testCompletedTLSFragment("PO", tlsDirectionSend)
	first.ConnectionID = 0x5151

	events, recognized := assembler.AddRecognized(first)
	if !recognized {
		t.Fatal("partial HTTP method prefix was not recognized")
	}
	if len(events) != 0 {
		t.Fatalf("partial prefix emitted %d events, want 0", len(events))
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("pending = %d, want 1", got)
	}

	second := testCompletedTLSFragment("ST /v1/messages HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 2\r\n\r\nok", tlsDirectionSend)
	second.ConnectionID = first.ConnectionID
	second.TimestampNS = first.TimestampNS + uint64(time.Millisecond)
	events, recognized = assembler.AddRecognized(second)
	if !recognized {
		t.Fatal("continuation of owned HTTP stream was not recognized")
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Method != "POST" || events[0].URL != "/v1/messages" {
		t.Fatalf("unexpected reassembled request: %+v", events[0])
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("pending = %d, want 0 after completion", got)
	}
}

func TestTLSHTTPStreamAssemblerDropsIncompleteTruncatedStream(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	fragment := testCompletedTLSFragment(
		"POST /large HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 100\r\n\r\nshort",
		tlsDirectionSend,
	)
	fragment.ConnectionID = 0x6161
	fragment.Flags = tlsFlagTruncated
	fragment.OriginalLen = uint32(len(fragment.Payload) + 4096)

	events, recognized := assembler.AddRecognized(fragment)
	if !recognized {
		t.Fatal("truncated HTTP stream was not recognized")
	}
	if len(events) != 0 {
		t.Fatalf("truncated incomplete stream emitted %d events, want 0", len(events))
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("pending = %d, want 0 after known capture gap", got)
	}
	if assembler.Dropped() == 0 {
		t.Fatal("expected known capture gap to increment dropped count")
	}

	// Bytes from the next TLS call must not be stitched onto the dropped body.
	tail := testCompletedTLSFragment(strings.Repeat("x", 95), tlsDirectionSend)
	tail.ConnectionID = fragment.ConnectionID
	tail.TimestampNS = fragment.TimestampNS + uint64(time.Millisecond)
	if events, recognized := assembler.AddRecognized(tail); recognized || len(events) != 0 {
		t.Fatalf("post-gap tail was unexpectedly owned/emitted: recognized=%v events=%d", recognized, len(events))
	}
}

func TestTLSHTTPStreamAssemblerDropsResidualAfterTruncatedRecord(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	payload := "GET /ok HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 0\r\n\r\n" +
		"POST /partial HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 100\r\n\r\nabc"
	fragment := testCompletedTLSFragment(payload, tlsDirectionSend)
	fragment.ConnectionID = 0x7171
	fragment.Flags = tlsFlagTruncated
	fragment.OriginalLen = uint32(len(payload) + 1024)

	events, recognized := assembler.AddRecognized(fragment)
	if !recognized {
		t.Fatal("truncated multi-message record was not recognized")
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want the complete first request only", len(events))
	}
	if events[0].URL != "/ok" {
		t.Fatalf("first URL = %q, want /ok", events[0].URL)
	}
	if !events[0].Truncated {
		t.Fatal("event from a truncated TLS record must remain conservatively marked truncated")
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("pending residual = %d, want 0 after capture gap", got)
	}
}

func TestTLSHTTPStreamAssemblerDoesNotOwnUnrecognizedPlaintext(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	fragment := testCompletedTLSFragment("SSH-2.0-example", tlsDirectionSend)
	if events, recognized := assembler.AddRecognized(fragment); recognized || len(events) != 0 {
		t.Fatalf("non-HTTP plaintext recognized=%v events=%d, want false/0", recognized, len(events))
	}
}
