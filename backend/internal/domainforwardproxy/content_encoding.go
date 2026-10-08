package domainforwardproxy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
)

func normalizeContentEncoding(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "identity" {
		return ""
	}
	return value
}

func bodyEncodingRewriteable(value string) bool {
	switch normalizeContentEncoding(value) {
	case "", "gzip":
		return true
	default:
		return false
	}
}

func decodeBodyForRewrite(encoding string, raw []byte, limit int64) ([]byte, bool, error) {
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
		decoded, readErr := io.ReadAll(io.LimitReader(reader, limit+1))
		closeErr := reader.Close()
		if readErr != nil {
			return nil, false, fmt.Errorf("read gzip body: %w", readErr)
		}
		if closeErr != nil {
			return nil, false, fmt.Errorf("close gzip body: %w", closeErr)
		}
		if int64(len(decoded)) > limit {
			return nil, false, nil
		}
		return decoded, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported content encoding %q", encoding)
	}
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
	default:
		return nil, fmt.Errorf("unsupported content encoding %q", encoding)
	}
}
