package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func testCompletedTLSFragment(payload string, direction uint8) completedTLSFragment {
	return completedTLSFragment{
		TimestampNS: uint64(time.Date(2026, time.May, 10, 12, 0, 0, 0, time.UTC).UnixNano()),
		PID:         4321,
		TGID:        8765,
		LibType:     tlsLibOpenSSL,
		Direction:   direction,
		Comm:        "curl",
		Payload:     []byte(payload),
	}
}

func TestParseTLSPlaintextHTTPRequestRedactsSensitiveHeaders(t *testing.T) {
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"POST /login HTTP/1.1",
		"Host: example.com",
		"Authorization: Bearer secret-token",
		"X-API-Key: abc123",
		"Cookie: session=super-secret",
		"Content-Type: application/json",
		"Content-Length: 22",
		"",
		`{"password":"hunter2"}`,
	}, "\r\n"), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if event.Method != "POST" {
		t.Fatalf("Method = %q, want POST", event.Method)
	}
	if event.URL != "/login" {
		t.Fatalf("URL = %q, want /login", event.URL)
	}
	if event.Host != "example.com" {
		t.Fatalf("Host = %q, want example.com", event.Host)
	}
	if got := event.Headers["authorization"]; got != "***REDACTED***" {
		t.Fatalf("authorization header = %q, want redacted", got)
	}
	if got := event.Headers["x-api-key"]; got != "***REDACTED***" {
		t.Fatalf("x-api-key header = %q, want redacted", got)
	}
	if got := event.Headers["cookie"]; got != "***REDACTED***" {
		t.Fatalf("cookie header = %q, want redacted", got)
	}
	if got := event.Body; !strings.Contains(got, "\n  \"password\": \"hunter2\"\n") {
		t.Fatalf("Body = %q, want pretty-printed JSON", got)
	}
	if event.RawHexDump != "" {
		t.Fatalf("RawHexDump = %q, want empty for parsed HTTP", event.RawHexDump)
	}
	if !event.RawAvailable {
		t.Fatalf("RawAvailable = false, want true for parsed HTTP")
	}
}

func TestParseTLSPlaintextHTTPResponse(t *testing.T) {
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"HTTP/1.1 201 Created",
		"Content-Type: application/json",
		"Set-Cookie: session=secret; HttpOnly",
		"Content-Length: 11",
		"",
		`{"ok":true}`,
	}, "\r\n"), tlsDirectionRecv)

	event := parseTLSPlaintext(fragment)

	if event.StatusCode != 201 {
		t.Fatalf("StatusCode = %d, want 201", event.StatusCode)
	}
	if got := event.Headers["set-cookie"]; got != "***REDACTED***" {
		t.Fatalf("set-cookie header = %q, want redacted", got)
	}
	if got := event.Body; !strings.Contains(got, "\n  \"ok\": true\n") {
		t.Fatalf("Body = %q, want pretty-printed JSON", got)
	}
	if event.Method != "" || event.URL != "" {
		t.Fatalf("unexpected request fields for response: method=%q url=%q", event.Method, event.URL)
	}
}

func TestParseTLSPlaintextNonHTTPUsesHexDump(t *testing.T) {
	fragment := completedTLSFragment{
		TimestampNS: uint64(time.Now().UnixNano()),
		PID:         1,
		TGID:        2,
		LibType:     tlsLibGo,
		Direction:   tlsDirectionSend,
		Comm:        "go-app",
		Payload: []byte(strings.Join([]string{
			"HELLO /not-http HTTP/1.1",
			"Header: value",
			"",
			"body",
		}, "\r\n")),
	}

	event := parseTLSPlaintext(fragment)

	if event.RawHexDump == "" {
		t.Fatalf("RawHexDump = %q, want hex dump", event.RawHexDump)
	}
	if event.Method != "" || event.URL != "" || len(event.Headers) != 0 {
		t.Fatalf("unexpected structured HTTP fields for non-HTTP payload: %+v", event)
	}
	if event.RawAvailable {
		t.Fatalf("RawAvailable = true, want false for non-HTTP payload")
	}
}

func TestParseTLSPlaintextRedactsProxyAuthorizationHeader(t *testing.T) {
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"POST /proxy HTTP/1.1",
		"Host: example.com",
		"Proxy-Authorization: Basic secret",
		"pRoXy-aUtHoRiZaTiOn: Digest another-secret",
		"Content-Length: 0",
		"",
		"",
	}, "\r\n"), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if got := event.Headers["proxy-authorization"]; got != "***REDACTED***" {
		t.Fatalf("proxy-authorization header = %q, want redacted", got)
	}
}

func TestParseTLSPlaintextTruncatesLargeBody(t *testing.T) {
	largeBody := "{" + strings.Repeat("\"a\":\"xxxxxxxxxx\",", 2000) + "\"z\":\"end\"}"
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"POST /bulk HTTP/1.1",
		"Host: example.com",
		"Content-Type: text/plain",
		"Content-Length: 25000",
		"",
		largeBody,
	}, "\r\n"), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if !event.Truncated {
		t.Fatalf("Truncated = false, want true")
	}
	if event.BodySize <= tlsMaxBodySize {
		t.Fatalf("BodySize = %d, want larger than max body size", event.BodySize)
	}
	if len(event.Body) != tlsMaxBodySize {
		t.Fatalf("Body length = %d, want %d", len(event.Body), tlsMaxBodySize)
	}
}

func TestParseTLSPlaintextTruncatesBasedOnRawBodySize(t *testing.T) {
	rawJSON := "[\n" + strings.Repeat(" ", tlsMaxBodySize+256) + "1\n]"
	fragment := testCompletedTLSFragment(fmt.Sprintf(
		"POST /raw-size HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
		len(rawJSON),
		rawJSON,
	), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if !event.Truncated {
		t.Fatalf("Truncated = false, want true when raw body exceeds limit")
	}
	if len(event.Body) > tlsMaxBodySize {
		t.Fatalf("Body length = %d, want at most %d", len(event.Body), tlsMaxBodySize)
	}
	if event.BodySize <= tlsMaxBodySize {
		t.Fatalf("BodySize = %d, want larger than max body size", event.BodySize)
	}
}

func TestParseTLSPlaintextBoundsBodyReadToMaxPlusOne(t *testing.T) {
	body := strings.Repeat("x", tlsMaxBodySize+512)
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"POST /bounded HTTP/1.1",
		"Host: example.com",
		"Content-Type: text/plain",
		"Content-Length: 999999",
		"",
		body,
	}, "\r\n"), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if event.BodySize != tlsMaxBodySize+1 {
		t.Fatalf("BodySize = %d, want %d", event.BodySize, tlsMaxBodySize+1)
	}
	if !event.Truncated {
		t.Fatalf("Truncated = false, want true")
	}
	if len(event.Body) != tlsMaxBodySize {
		t.Fatalf("Body length = %d, want %d", len(event.Body), tlsMaxBodySize)
	}
}
