package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

func expandRemoteDatasetPayloads(data []byte, contentType, source string, depth int) ([]remoteDatasetPayload, error) {
	if depth > 4 {
		return []remoteDatasetPayload{{Source: source, ContentType: contentType, Data: data}}, nil
	}
	if isZipPayload(data, contentType, source) {
		return expandZipRemoteDatasetPayload(data, source, depth)
	}
	if isTarPayload(data, contentType, source) {
		return expandTarRemoteDatasetPayload(data, source, depth)
	}
	if isGzipPayload(data, contentType, source) {
		decompressed, err := gunzipRemoteDatasetPayload(data)
		if err != nil {
			return nil, err
		}
		return expandRemoteDatasetPayloads(decompressed, "", stripCompressionSuffix(source), depth+1)
	}
	if isBzip2Payload(data, contentType, source) {
		decompressed, err := bunzip2RemoteDatasetPayload(data)
		if err != nil {
			return nil, err
		}
		return expandRemoteDatasetPayloads(decompressed, "", stripCompressionSuffix(source), depth+1)
	}
	if isXzPayload(data, contentType, source) {
		decompressed, err := unxzRemoteDatasetPayload(data)
		if err != nil {
			return nil, err
		}
		return expandRemoteDatasetPayloads(decompressed, "", stripCompressionSuffix(source), depth+1)
	}
	return []remoteDatasetPayload{{Source: source, ContentType: contentType, Data: data}}, nil
}

func expandZipRemoteDatasetPayload(data []byte, source string, depth int) ([]remoteDatasetPayload, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	payloads := make([]remoteDatasetPayload, 0, len(reader.File))
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if shouldSkipArchiveMember(file.Name) {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			continue
		}
		fileData, readErr := io.ReadAll(io.LimitReader(rc, remoteDatasetFetchLimitBytes+1))
		_ = rc.Close()
		if readErr != nil {
			continue
		}
		if len(fileData) > remoteDatasetFetchLimitBytes {
			return nil, fmt.Errorf("extracted file %q is larger than %d bytes", file.Name, remoteDatasetFetchLimitBytes)
		}
		nextSource := joinDatasetSource(source, file.Name)
		nested, err := expandRemoteDatasetPayloads(fileData, "", nextSource, depth+1)
		if err != nil {
			continue
		}
		payloads = append(payloads, nested...)
	}
	if len(payloads) == 0 {
		return nil, fmt.Errorf("zip archive %q did not contain any extractable dataset files", source)
	}
	return payloads, nil
}

func expandTarRemoteDatasetPayload(data []byte, source string, depth int) ([]remoteDatasetPayload, error) {
	reader := tar.NewReader(bytes.NewReader(data))
	payloads := make([]remoteDatasetPayload, 0)
	for {
		hdr, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			continue
		}
		if hdr == nil || hdr.FileInfo().IsDir() {
			continue
		}
		if shouldSkipArchiveMember(hdr.Name) {
			continue
		}
		if hdr.Size > remoteDatasetFetchLimitBytes {
			return nil, fmt.Errorf("extracted file %q is larger than %d bytes", hdr.Name, remoteDatasetFetchLimitBytes)
		}
		fileData, err := io.ReadAll(io.LimitReader(reader, remoteDatasetFetchLimitBytes+1))
		if err != nil {
			continue
		}
		nextSource := joinDatasetSource(source, hdr.Name)
		nested, err := expandRemoteDatasetPayloads(fileData, "", nextSource, depth+1)
		if err != nil {
			continue
		}
		payloads = append(payloads, nested...)
	}
	if len(payloads) == 0 {
		return nil, fmt.Errorf("tar archive %q did not contain any extractable dataset files", source)
	}
	return payloads, nil
}

func gunzipRemoteDatasetPayload(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, remoteDatasetFetchLimitBytes+1))
}

func bunzip2RemoteDatasetPayload(data []byte) ([]byte, error) {
	return io.ReadAll(io.LimitReader(bzip2.NewReader(bytes.NewReader(data)), remoteDatasetFetchLimitBytes+1))
}

func unxzRemoteDatasetPayload(data []byte) ([]byte, error) {
	reader, err := xz.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(reader, remoteDatasetFetchLimitBytes+1))
}

func isZipPayload(data []byte, contentType, source string) bool {
	if len(data) >= 4 && bytes.Equal(data[:4], []byte("PK\x03\x04")) {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(ct, "zip") {
		return true
	}
	lower := strings.ToLower(source)
	return strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".jar") || strings.HasSuffix(lower, ".war")
}

func isTarPayload(data []byte, contentType, source string) bool {
	if len(data) >= 262 && bytes.Equal(data[257:262], []byte("ustar")) {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(ct, "tar") {
		return true
	}
	lower := strings.ToLower(source)
	return strings.HasSuffix(lower, ".tar")
}

func isGzipPayload(data []byte, contentType, source string) bool {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(ct, "gzip") || strings.Contains(ct, "x-gzip") {
		return true
	}
	lower := strings.ToLower(source)
	return strings.HasSuffix(lower, ".gz") || strings.HasSuffix(lower, ".tgz") || strings.HasSuffix(lower, ".tar.gz")
}

func isBzip2Payload(data []byte, contentType, source string) bool {
	if len(data) >= 3 && bytes.Equal(data[:3], []byte("BZh")) {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(ct, "bzip2") || strings.Contains(ct, "x-bzip2") {
		return true
	}
	lower := strings.ToLower(source)
	return strings.HasSuffix(lower, ".bz2") || strings.HasSuffix(lower, ".tbz2") || strings.HasSuffix(lower, ".tbz")
}

func isXzPayload(data []byte, contentType, source string) bool {
	if len(data) >= 6 && bytes.Equal(data[:6], []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}) {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(ct, "x-xz") || strings.Contains(ct, "xz") {
		return true
	}
	lower := strings.ToLower(source)
	return strings.HasSuffix(lower, ".xz") || strings.HasSuffix(lower, ".txz")
}

func stripCompressionSuffix(source string) string {
	lower := strings.ToLower(strings.TrimSpace(source))
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		return source[:len(source)-3]
	case strings.HasSuffix(lower, ".tar.bz2"):
		return source[:len(source)-4]
	case strings.HasSuffix(lower, ".tgz"), strings.HasSuffix(lower, ".tbz2"), strings.HasSuffix(lower, ".tbz"), strings.HasSuffix(lower, ".txz"):
		if idx := strings.LastIndex(source, "."); idx > 0 {
			return source[:idx] + ".tar"
		}
	case strings.HasSuffix(lower, ".gz"):
		return source[:len(source)-3]
	case strings.HasSuffix(lower, ".bz2"):
		return source[:len(source)-4]
	case strings.HasSuffix(lower, ".xz"):
		return source[:len(source)-3]
	}
	return source
}

func joinDatasetSource(parent, child string) string {
	parent = strings.TrimSpace(parent)
	child = strings.TrimSpace(child)
	switch {
	case parent == "":
		return child
	case child == "":
		return parent
	default:
		return parent + "!" + child
	}
}

func shouldSkipArchiveMember(name string) bool {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(name)))
	if base == "" {
		return true
	}
	switch {
	case strings.HasPrefix(base, "readme"),
		strings.HasPrefix(base, "license"),
		strings.HasPrefix(base, "notice"),
		strings.HasPrefix(base, "changelog"),
		strings.HasPrefix(base, "copying"):
		return true
	case strings.HasSuffix(base, ".md"),
		strings.HasSuffix(base, ".rst"),
		strings.HasSuffix(base, ".html"),
		strings.HasSuffix(base, ".htm"),
		strings.HasSuffix(base, ".pdf"),
		strings.HasSuffix(base, ".png"),
		strings.HasSuffix(base, ".jpg"),
		strings.HasSuffix(base, ".jpeg"),
		strings.HasSuffix(base, ".gif"),
		strings.HasSuffix(base, ".svg"),
		strings.HasSuffix(base, ".exe"),
		strings.HasSuffix(base, ".dll"),
		strings.HasSuffix(base, ".so"),
		strings.HasSuffix(base, ".o"),
		strings.HasSuffix(base, ".a"),
		strings.HasSuffix(base, ".pyc"),
		strings.HasSuffix(base, ".class"),
		strings.HasSuffix(base, ".zip"),
		strings.HasSuffix(base, ".gz"),
		strings.HasSuffix(base, ".tar"),
		strings.HasSuffix(base, ".bz2"),
		strings.HasSuffix(base, ".xz"):
		return true
	}
	return false
}

func mergeDatasetFormat(current, next string) string {
	current = strings.ToLower(strings.TrimSpace(current))
	next = strings.ToLower(strings.TrimSpace(next))
	if next == "" {
		return current
	}
	if current == "" {
		return next
	}
	if current == next {
		return current
	}
	return "archive"
}

func looksLikeHTMLDataset(raw []byte, contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") {
		return true
	}

	if isBinary(raw) {
		return false
	}

	// Only check the first few hundred bytes for common HTML tags
	checkLen := len(raw)
	if checkLen > 1024 {
		checkLen = 1024
	}
	trimmed := strings.ToLower(strings.TrimSpace(string(raw[:checkLen])))
	return strings.HasPrefix(trimmed, "<!doctype html") ||
		strings.HasPrefix(trimmed, "<html") ||
		strings.HasPrefix(trimmed, "<body")
}
