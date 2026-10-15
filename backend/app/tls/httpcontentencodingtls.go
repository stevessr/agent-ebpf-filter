package tls

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// formatEncodedTLSPlaintextBody decodes captured HTTP Content-Encoding before
// formatting and redacting it. This is a display-only operation: neither the
// intercepted wire bytes nor the original captured byte counts are changed.
//
// Malformed, unsupported or incomplete compressed payloads are never fed to
// text redaction as binary data. A bounded placeholder is returned instead.
func formatEncodedTLSPlaintextBody(body []byte, contentType, contentEncoding string) (string, bool) {
	encoding := strings.ToLower(strings.TrimSpace(contentEncoding))
	if encoding == "" || encoding == "identity" {
		formatted, truncated := formatTLSPlaintextBody(body, contentType)
		return sanitizeTLSBody(formatted, contentType), truncated
	}

	var reader io.Reader
	switch encoding {
	case "gzip":
		gz, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return "[compressed HTTP body unavailable for preview]", true
		}
		defer gz.Close()
		reader = gz
	case "br":
		reader = brotli.NewReader(bytes.NewReader(body))
	case "zstd":
		decoder, err := zstd.NewReader(bytes.NewReader(body),
			zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(1<<20))
		if err != nil {
			return "[compressed HTTP body unavailable for preview]", true
		}
		defer decoder.Close()
		reader = decoder
	default:
		// Unknown and stacked encodings are not safely displayable.
		return "[unsupported HTTP content encoding; body hidden]", true
	}

	decoded, err := io.ReadAll(io.LimitReader(reader, tlsMaxBodySize+1))
	if err != nil {
		// The eBPF buffer may contain an incomplete compressed frame.
		return "[compressed HTTP body unavailable for preview]", true
	}
	formatted, truncated := formatTLSPlaintextBody(decoded, contentType)
	return sanitizeTLSBody(formatted, contentType), truncated || len(decoded) > tlsMaxBodySize
}
