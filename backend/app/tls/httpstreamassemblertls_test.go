package tls

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---- moved from backend/zz_merged_backend_test.go section httpstreamassemblertls_test.go ----

func TestTLSHTTPStreamAssemblerMergesSplitHTTPResponse(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	body := `{"ok":true,"message":"merged"}`
	payload := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	first := testCompletedTLSFragment(payload[:70], tlsDirectionRecv)
	second := testCompletedTLSFragment(payload[70:], tlsDirectionRecv)
	second.TimestampNS = first.TimestampNS + uint64(time.Millisecond)

	if got := assembler.Add(first); len(got) != 0 {
		t.Fatalf("first fragment emitted %d events, want 0", len(got))
	}
	events := assembler.Add(second)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	event := events[0]
	if event.Type != "http_response" {
		t.Fatalf("Type = %q, want http_response", event.Type)
	}
	if event.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, want 200", event.StatusCode)
	}
	if !strings.Contains(event.Body, `"message": "merged"`) {
		t.Fatalf("Body = %q, want merged JSON body", event.Body)
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("pending = %d, want 0", got)
	}
}

func TestTLSHTTPStreamAssemblerMergesSplitHTTPRequest(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	body := `{"prompt":"hello"}`
	payload := fmt.Sprintf("POST /v1/messages HTTP/1.1\r\nHost: api.example.com\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	first := testCompletedTLSFragment(payload[:55], tlsDirectionSend)
	second := testCompletedTLSFragment(payload[55:], tlsDirectionSend)
	second.TimestampNS = first.TimestampNS + uint64(time.Millisecond)

	if got := assembler.Add(first); len(got) != 0 {
		t.Fatalf("first fragment emitted %d events, want 0", len(got))
	}
	events := assembler.Add(second)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	event := events[0]
	if event.Type != "http_request" || event.Method != "POST" || event.URL != "/v1/messages" || event.Host != "api.example.com" {
		t.Fatalf("unexpected request event: %+v", event)
	}
	if !strings.Contains(event.Body, `"prompt": "hello"`) {
		t.Fatalf("Body = %q, want pretty JSON", event.Body)
	}
}

func TestTLSHTTPStreamAssemblerSeparatesConcurrentConnections(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	payloadOne := "POST /one HTTP/1.1\r\nHost: one.example\r\nContent-Length: 3\r\n\r\none"
	payloadTwo := "POST /two HTTP/1.1\r\nHost: two.example\r\nContent-Length: 3\r\n\r\ntwo"
	const split = 30

	oneA := testCompletedTLSFragment(payloadOne[:split], tlsDirectionSend)
	oneA.ConnectionID = 0x1111
	oneB := testCompletedTLSFragment(payloadOne[split:], tlsDirectionSend)
	oneB.ConnectionID = oneA.ConnectionID
	oneB.TimestampNS = oneA.TimestampNS + uint64(2*time.Millisecond)

	twoA := testCompletedTLSFragment(payloadTwo[:split], tlsDirectionSend)
	twoA.ConnectionID = 0x2222
	twoA.TimestampNS = oneA.TimestampNS + uint64(time.Millisecond)
	twoB := testCompletedTLSFragment(payloadTwo[split:], tlsDirectionSend)
	twoB.ConnectionID = twoA.ConnectionID
	twoB.TimestampNS = oneA.TimestampNS + uint64(3*time.Millisecond)

	if events := assembler.Add(oneA); len(events) != 0 {
		t.Fatalf("connection one first half emitted %d events", len(events))
	}
	if events := assembler.Add(twoA); len(events) != 0 {
		t.Fatalf("connection two first half emitted %d events", len(events))
	}
	oneEvents := assembler.Add(oneB)
	if len(oneEvents) != 1 || oneEvents[0].URL != "/one" || oneEvents[0].Host != "one.example" {
		t.Fatalf("connection one was cross-contaminated: %+v", oneEvents)
	}
	twoEvents := assembler.Add(twoB)
	if len(twoEvents) != 1 || twoEvents[0].URL != "/two" || twoEvents[0].Host != "two.example" {
		t.Fatalf("connection two was cross-contaminated: %+v", twoEvents)
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("pending = %d, want 0", got)
	}
}

func TestTLSHTTPStreamAssemblerDropsRawTLSRecords(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	fragment := testCompletedTLSFragment("not http plaintext", tlsDirectionRecv)

	if events := assembler.Add(fragment); len(events) != 0 {
		t.Fatalf("events = %d, want 0", len(events))
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("pending = %d, want 0", got)
	}
}

func TestTLSHTTPStreamAssemblerCopiesRequestContextToResponse(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	request := testCompletedTLSFragment("GET /health HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 0\r\n\r\n", tlsDirectionSend)
	request.ConnectionID = 0x1234
	response := testCompletedTLSFragment("HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n", tlsDirectionRecv)
	response.ConnectionID = request.ConnectionID
	response.TimestampNS = request.TimestampNS + uint64(time.Millisecond)

	if events := assembler.Add(request); len(events) != 1 {
		t.Fatalf("request events = %d, want 1", len(events))
	}
	events := assembler.Add(response)
	if len(events) != 1 {
		t.Fatalf("response events = %d, want 1", len(events))
	}
	event := events[0]
	if event.Host != "api.example.com" || event.Method != "GET" || event.URL != "/health" {
		t.Fatalf("response context = method %q url %q host %q, want request context", event.Method, event.URL, event.Host)
	}
}

func TestTLSHTTPStreamAssemblerParsesChunkedResponseAfterTerminator(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	payload := strings.Join([]string{
		"HTTP/1.1 200 OK",
		"Content-Type: text/plain",
		"Transfer-Encoding: chunked",
		"",
		"5",
		"hello",
		"6",
		" world",
		"0",
		"",
		"",
	}, "\r\n")
	first := testCompletedTLSFragment(payload[:90], tlsDirectionRecv)
	second := testCompletedTLSFragment(payload[90:], tlsDirectionRecv)
	second.TimestampNS = first.TimestampNS + uint64(time.Millisecond)

	if events := assembler.Add(first); len(events) != 0 {
		t.Fatalf("first events = %d, want 0", len(events))
	}
	events := assembler.Add(second)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if got := events[0].Body; got != "hello world" {
		t.Fatalf("Body = %q, want dechunked body", got)
	}
}
