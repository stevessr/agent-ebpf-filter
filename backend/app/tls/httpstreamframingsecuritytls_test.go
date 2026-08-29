package tls

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestTLSHTTPStreamRejectsConflictingContentLengths(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	payload := "POST /smuggle HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 4\r\nContent-Length: 8\r\n\r\ntestxxxx"
	fragment := testCompletedTLSFragment(payload, tlsDirectionSend)
	fragment.ConnectionID = 0x8101

	events, recognized := assembler.AddRecognized(fragment)
	if !recognized {
		t.Fatal("ambiguous HTTP request should remain owned for raw suppression")
	}
	if len(events) != 0 {
		t.Fatalf("ambiguous request emitted %d events, want 0", len(events))
	}
	if assembler.Pending() != 0 {
		t.Fatal("ambiguous request must not remain pending")
	}
	if assembler.Dropped() == 0 {
		t.Fatal("ambiguous request should increment dropped count")
	}
}

func TestTLSHTTPStreamAllowsIdenticalRepeatedContentLength(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	payload := "POST /ok HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 4\r\nContent-Length: 4\r\n\r\ntest"
	fragment := testCompletedTLSFragment(payload, tlsDirectionSend)
	fragment.ConnectionID = 0x8102

	events, recognized := assembler.AddRecognized(fragment)
	if !recognized {
		t.Fatal("valid repeated Content-Length request was not recognized")
	}
	if len(events) != 1 || events[0].URL != "/ok" || events[0].Body != "test" {
		t.Fatalf("unexpected valid repeated Content-Length result: %+v", events)
	}
}

func TestTLSHTTPStreamRejectsTransferEncodingWithContentLength(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	payload := strings.Join([]string{
		"POST /smuggle HTTP/1.1",
		"Host: api.example.com",
		"Transfer-Encoding: chunked",
		"Content-Length: 4",
		"",
		"4",
		"test",
		"0",
		"",
		"",
	}, "\r\n")
	fragment := testCompletedTLSFragment(payload, tlsDirectionSend)
	fragment.ConnectionID = 0x8103

	if events, recognized := assembler.AddRecognized(fragment); !recognized || len(events) != 0 {
		t.Fatalf("TE+CL recognized=%v events=%d, want true/0", recognized, len(events))
	}
	if assembler.Pending() != 0 {
		t.Fatal("TE+CL ambiguity must not remain pending")
	}
}

func TestTLSHTTPStreamRejectsAmbiguousTransferEncodingOrder(t *testing.T) {
	cases := []string{
		"Transfer-Encoding: chunked, gzip",
		"Transfer-Encoding: chunked, chunked",
	}
	for _, header := range cases {
		t.Run(header, func(t *testing.T) {
			assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
			payload := "POST /bad HTTP/1.1\r\nHost: api.example.com\r\n" + header + "\r\n\r\n0\r\n\r\n"
			fragment := testCompletedTLSFragment(payload, tlsDirectionSend)
			fragment.ConnectionID = 0x8104
			if events, recognized := assembler.AddRecognized(fragment); !recognized || len(events) != 0 {
				t.Fatalf("ambiguous TE recognized=%v events=%d, want true/0", recognized, len(events))
			}
		})
	}
}

func TestTLSHTTPStreamAllowsTransferCodingEndingInChunked(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	payload := "POST /encoded HTTP/1.1\r\nHost: api.example.com\r\nTransfer-Encoding: gzip, chunked\r\n\r\n4\r\ntest\r\n0\r\n\r\n"
	fragment := testCompletedTLSFragment(payload, tlsDirectionSend)
	fragment.ConnectionID = 0x8105

	events, recognized := assembler.AddRecognized(fragment)
	if !recognized {
		t.Fatal("valid final chunked transfer coding was not recognized")
	}
	if len(events) != 1 || events[0].URL != "/encoded" {
		t.Fatalf("unexpected final-chunked result: %+v", events)
	}
}

func TestTLSHTTPStreamRejectsObsFoldAndHeaderNameWhitespace(t *testing.T) {
	cases := []string{
		"POST /fold HTTP/1.1\r\nHost: api.example.com\r\nX-Test: one\r\n two\r\nContent-Length: 0\r\n\r\n",
		"POST /space HTTP/1.1\r\nHost: api.example.com\r\nContent-Length : 0\r\n\r\n",
	}
	for index, payload := range cases {
		assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
		fragment := testCompletedTLSFragment(payload, tlsDirectionSend)
		fragment.ConnectionID = uint64(0x8200 + index)
		if events, recognized := assembler.AddRecognized(fragment); !recognized || len(events) != 0 {
			t.Fatalf("case %d recognized=%v events=%d, want true/0", index, recognized, len(events))
		}
	}
}

func TestTLSHTTPStreamRejectsOversizedHeaderBeforeBufferGrowth(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	payload := "POST /huge HTTP/1.1\r\nHost: api.example.com\r\nX-Fill: " + strings.Repeat("x", tlsHTTPMaxHeaderBytes) // no terminator
	fragment := testCompletedTLSFragment(payload, tlsDirectionSend)
	fragment.ConnectionID = 0x8301

	if events, recognized := assembler.AddRecognized(fragment); !recognized || len(events) != 0 {
		t.Fatalf("oversized header recognized=%v events=%d, want true/0", recognized, len(events))
	}
	if assembler.Pending() != 0 {
		t.Fatal("oversized header must not be retained")
	}
}

func TestTLSHTTPStreamRejectsImpossibleContentLengthEarly(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	payload := fmt.Sprintf("POST /huge HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: %d\r\n\r\n", tlsHTTPStreamMaxBuffer+1)
	fragment := testCompletedTLSFragment(payload, tlsDirectionSend)
	fragment.ConnectionID = 0x8302

	if events, recognized := assembler.AddRecognized(fragment); !recognized || len(events) != 0 {
		t.Fatalf("impossible CL recognized=%v events=%d, want true/0", recognized, len(events))
	}
	if assembler.Pending() != 0 {
		t.Fatal("impossible Content-Length must not be retained")
	}
}

func TestTLSHTTPStreamValidatesSecondPipelinedMessage(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	first := "GET /first HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 0\r\n\r\n"
	second := "POST /second HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 1\r\nContent-Length: 2\r\n\r\nxx"
	fragment := testCompletedTLSFragment(first+second, tlsDirectionSend)
	fragment.ConnectionID = 0x8401

	if events, recognized := assembler.AddRecognized(fragment); !recognized || len(events) != 0 {
		t.Fatalf("ambiguous pipelined capture recognized=%v events=%d, want true/0 fail-closed", recognized, len(events))
	}
}
