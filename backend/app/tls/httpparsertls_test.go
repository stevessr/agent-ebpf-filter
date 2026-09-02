package tls

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// ---- moved from backend/zz_merged_backend_test.go section httpparsertls_test.go ----

func testCompletedTLSFragment(payload string, direction uint8) CompletedTLSFragment {
	return CompletedTLSFragment{
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
	if event.Body != "{\n  \"password\": \"***REDACTED***\"\n}" {
		t.Fatalf("Body = %q, want redacted pretty-printed JSON", event.Body)
	}
	if event.RawHexDump != "" {
		t.Fatalf("RawHexDump = %q, want empty for parsed HTTP", event.RawHexDump)
	}
	if !event.RawAvailable {
		t.Fatalf("RawAvailable = false, want true for parsed HTTP")
	}
}

func TestParseTLSPlaintextHTTPRequestRedactsSensitiveURLQuery(t *testing.T) {
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"GET /v1/messages?api_key=abc&token=secret&safe=value HTTP/1.1",
		"Host: example.com",
		"Content-Length: 0",
		"",
		"",
	}, "\r\n"), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if strings.Contains(event.URL, "abc") || strings.Contains(event.URL, "secret") {
		t.Fatalf("URL = %q, want sensitive query values redacted", event.URL)
	}
	if !strings.Contains(event.URL, "api_key=%2A%2A%2AREDACTED%2A%2A%2A") || !strings.Contains(event.URL, "safe=value") {
		t.Fatalf("URL = %q, want redacted sensitive query and preserved safe query", event.URL)
	}
}

func TestParseTLSPlaintextSSEAnnotatesDigest(t *testing.T) {
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"HTTP/1.1 200 OK",
		"Content-Type: text/event-stream",
		"Content-Length: 39",
		"",
		"event: completion\ndata: token=secret\n\n",
	}, "\r\n"), tlsDirectionRecv)

	event := parseTLSPlaintext(fragment)

	if event.Type != "sse_message" {
		t.Fatalf("Type = %q, want sse_message", event.Type)
	}
	if event.SSEEvent != "completion" {
		t.Fatalf("SSEEvent = %q, want completion", event.SSEEvent)
	}
	if event.SSEDataDigest == "" {
		t.Fatalf("SSEDataDigest empty, want digest")
	}
	if strings.Contains(event.Body, "secret") {
		t.Fatalf("Body = %q, want inline secret redacted", event.Body)
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
	fragment := CompletedTLSFragment{
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

func TestParseTLSPlaintextPreservesBodyLargerThanOldWindow(t *testing.T) {
	body := strings.Repeat("x", 24*1024)
	fragment := testCompletedTLSFragment(fmt.Sprintf(
		"POST /expanded HTTP/1.1\r\nHost: example.com\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
		len(body),
		body,
	), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)
	if event.Truncated {
		t.Fatalf("Truncated = true, want false for 24 KiB body inside expanded window")
	}
	if event.BodySize != len(body) || len(event.Body) != len(body) {
		t.Fatalf("body sizes captured=%d displayed=%d want=%d", event.BodySize, len(event.Body), len(body))
	}
}

func TestParseTLSPlaintextTruncatesLargeBody(t *testing.T) {
	largeBody := "{" + strings.Repeat("\"a\":\"xxxxxxxxxx\",", 2000) + "\"z\":\"end\"}"
	fragment := testCompletedTLSFragment(fmt.Sprintf(
		"POST /bulk HTTP/1.1\r\nHost: example.com\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
		len(largeBody),
		largeBody,
	), tlsDirectionSend)

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

func TestParseTLSPlaintextPreservesKernelCaptureTruncation(t *testing.T) {
	payload := "POST /small HTTP/1.1\r\nHost: example.com\r\nContent-Length: 2\r\n\r\nok"
	fragment := testCompletedTLSFragment(payload, tlsDirectionSend)
	fragment.TotalLen = uint32(len(payload))
	fragment.OriginalLen = uint32(len(payload) + 4096)
	fragment.Flags = tlsFlagTruncated

	event := parseTLSPlaintext(fragment)
	if !event.Truncated {
		t.Fatal("Truncated = false, want kernel capture truncation to survive HTTP formatting")
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

func TestParseTLSPlaintextTruncationKeepsUTF8Valid(t *testing.T) {
	body := strings.Repeat("a", tlsMaxBodySize-1) + "鱼"
	fragment := testCompletedTLSFragment(fmt.Sprintf(
		"POST /utf8 HTTP/1.1\r\nHost: example.com\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: %d\r\n\r\n%s",
		len(body),
		body,
	), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)
	if !event.Truncated {
		t.Fatal("Truncated = false, want true at UTF-8 boundary")
	}
	if len(event.Body) > tlsMaxBodySize {
		t.Fatalf("Body length = %d, want <= %d", len(event.Body), tlsMaxBodySize)
	}
	if !utf8.ValidString(event.Body) {
		t.Fatalf("truncated body is invalid UTF-8")
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
