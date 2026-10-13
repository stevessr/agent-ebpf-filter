package tls

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

func encodeTLSHTTPBodyTest(t *testing.T, encoding string, plain []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	switch encoding {
	case "gzip":
		writer := gzip.NewWriter(&out)
		if _, err := writer.Write(plain); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	case "br":
		writer := brotli.NewWriter(&out)
		if _, err := writer.Write(plain); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	case "zstd":
		writer, err := zstd.NewWriter(&out, zstd.WithEncoderConcurrency(1))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(plain); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unexpected test encoding %s", encoding)
	}
	return out.Bytes()
}

func TestTLSCompressedHTTPBodiesAreRedacted(t *testing.T) {
	const plain = `{"token":"super-secret","value":"ok"}`
	for _, encoding := range []string{"gzip", "br", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			wire := encodeTLSHTTPBodyTest(t, encoding, []byte(plain))
			for _, request := range []bool{false, true} {
				var headers string
				direction := tlsDirectionRecv
				if request {
					direction = tlsDirectionSend
					headers = fmt.Sprintf("POST /v1/chat HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\nContent-Encoding: %s\r\nContent-Length: %d\r\n\r\n", encoding, len(wire))
				} else {
					headers = fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Encoding: %s\r\nContent-Length: %d\r\n\r\n", encoding, len(wire))
				}
				fragment := testCompletedTLSFragment("", direction)
				fragment.Payload = append([]byte(headers), wire...)
				event := parseTLSPlaintext(fragment)
				if event.Type == "raw" {
					t.Fatalf("HTTP payload was not recognized: %+v", event)
				}
				if strings.Contains(event.Body, "super-secret") || !strings.Contains(event.Body, "***REDACTED***") {
					t.Fatalf("decoded body was not redacted: %q", event.Body)
				}
				if event.BodySize != len(wire) {
					t.Fatalf("wire byte count %d != %d", event.BodySize, len(wire))
				}
				if event.Truncated {
					t.Fatalf("small compressed body should not be truncated: %q", event.Body)
				}
			}
		})
	}
}

func TestTLSCompressedSSECanBeSummarized(t *testing.T) {
	wire := encodeTLSHTTPBodyTest(t, "br", []byte("event: message\ndata: token=private\n\n"))
	headers := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Encoding: br\r\nContent-Length: %d\r\n\r\n", len(wire))
	fragment := testCompletedTLSFragment("", tlsDirectionRecv)
	fragment.Payload = append([]byte(headers), wire...)
	event := parseTLSPlaintext(fragment)
	if event.Type != "sse_message" || event.SSEEvent != "message" || event.SSEDataDigest == "" {
		t.Fatalf("compressed SSE metadata missing: %+v", event)
	}
	if strings.Contains(event.Body, "private") {
		t.Fatalf("SSE not redacted: %q", event.Body)
	}
}

func TestTLSCompressedBodyLimitsAndMalformedPayloads(t *testing.T) {
	for _, encoding := range []string{"gzip", "br", "zstd"} {
		wire := encodeTLSHTTPBodyTest(t, encoding, bytes.Repeat([]byte("A"), tlsMaxBodySize*3))
		formatted, truncated := formatEncodedTLSPlaintextBody(wire, "text/plain", encoding)
		if !truncated || len(formatted) > tlsMaxBodySize {
			t.Fatalf("%s decoded payload not bounded: truncated=%t bytes=%d", encoding, truncated, len(formatted))
		}
	}
	formatted, truncated := formatEncodedTLSPlaintextBody([]byte("not a compressed stream"), "application/json", "gzip")
	if !truncated || !strings.Contains(formatted, "unavailable") {
		t.Fatalf("corrupt body should hide bytes: %q truncated=%v", formatted, truncated)
	}
	formatted, truncated = formatEncodedTLSPlaintextBody([]byte("password=secret"), "text/plain", "gzip, br")
	if !truncated || !strings.Contains(formatted, "unsupported") || strings.Contains(formatted, "secret") {
		t.Fatalf("unknown stacked encoding should not expose raw body: %q truncated=%v", formatted, truncated)
	}
}
