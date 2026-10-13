package domainforwardproxy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// Keep the decoder bounded even if an upstream sends a tiny compressed body
// with an enormous declared Zstandard window. Most API payloads need far less.
const minimumZstdDecoderMemory = 1 << 20

func normalizeContentEncoding(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "identity" {
		return ""
	}
	return value
}

func bodyEncodingRewriteable(value string) bool {
	switch normalizeContentEncoding(value) {
	case "", "gzip", "br", "zstd":
		return true
	default:
		// Stacked or unknown encodings must remain byte-for-byte untouched.
		return false
	}
}

func decodeBodyForRewrite(encoding string, raw []byte, limit int64) ([]byte, bool, error) {
	if limit < 0 {
		return nil, false, fmt.Errorf("invalid body rewrite limit %d", limit)
	}
	switch normalizeContentEncoding(encoding) {
	case "":
		if int64(len(raw)) > limit {
			return nil, false, nil
		}
		return raw, true, nil
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, false, fmt.Errorf("open gzip body: %w", err)
		}
		defer reader.Close()
		return readDecodedBodyBounded(reader, limit)
	case "br":
		return readDecodedBodyBounded(brotli.NewReader(bytes.NewReader(raw)), limit)
	case "zstd":
		// The frame may request a very large window despite a small wire size.
		// Bound both decoder concurrency and its in-memory window allowance.
		maxMemory := uint64(limit) + 1
		if maxMemory < minimumZstdDecoderMemory {
			maxMemory = minimumZstdDecoderMemory
		}
		reader, err := zstd.NewReader(bytes.NewReader(raw),
			zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(maxMemory))
		if err != nil {
			return nil, false, fmt.Errorf("open zstd body: %w", err)
		}
		defer reader.Close()
		return readDecodedBodyBounded(reader, limit)
	default:
		return nil, false, fmt.Errorf("unsupported content encoding %q", encoding)
	}
}

func readDecodedBodyBounded(reader io.Reader, limit int64) ([]byte, bool, error) {
	decoded, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, false, fmt.Errorf("read compressed HTTP body: %w", err)
	}
	if int64(len(decoded)) > limit {
		// Retain the original wire body rather than rewriting a partial decode.
		return nil, false, nil
	}
	return decoded, true, nil
}

func encodeBodyAfterRewrite(encoding string, decoded []byte) ([]byte, error) {
	switch normalizeContentEncoding(encoding) {
	case "":
		return decoded, nil
	case "gzip":
		var out bytes.Buffer
		writer := gzip.NewWriter(&out)
		if _, err := writer.Write(decoded); err != nil {
			return nil, fmt.Errorf("write gzip body: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("close gzip body: %w", err)
		}
		return out.Bytes(), nil
	case "br":
		var out bytes.Buffer
		writer := brotli.NewWriterLevel(&out, 4)
		if _, err := writer.Write(decoded); err != nil {
			return nil, fmt.Errorf("write brotli body: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("close brotli body: %w", err)
		}
		return out.Bytes(), nil
	case "zstd":
		var out bytes.Buffer
		writer, err := zstd.NewWriter(&out,
			zstd.WithEncoderConcurrency(1), zstd.WithEncoderLevel(zstd.SpeedFastest))
		if err != nil {
			return nil, fmt.Errorf("open zstd encoder: %w", err)
		}
		if _, err := writer.Write(decoded); err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("write zstd body: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("close zstd body: %w", err)
		}
		return out.Bytes(), nil
	default:
		return nil, fmt.Errorf("unsupported content encoding %q", encoding)
	}
}
