package app

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

// ---- moved from backend/zz_merged_backend.go section remotedatasetsarchive.go ----

const (
	remoteDatasetArchiveExpandedLimitBytes int64 = 64 << 20
	remoteDatasetArchiveMemberLimit              = 4096
	remoteDatasetArchiveDepthLimit               = 4
)

var errRemoteDatasetArchiveBudgetExceeded = errors.New("remote dataset archive budget exceeded")

type remoteDatasetArchiveBudget struct {
	maxBytes    int64
	usedBytes   int64
	maxMembers  int
	usedMembers int
	maxDepth    int
}

func newRemoteDatasetArchiveBudget(maxBytes int64, maxMembers, maxDepth int) *remoteDatasetArchiveBudget {
	return &remoteDatasetArchiveBudget{
		maxBytes:   maxBytes,
		maxMembers: maxMembers,
		maxDepth:   maxDepth,
	}
}

func newDefaultRemoteDatasetArchiveBudget() *remoteDatasetArchiveBudget {
	return newRemoteDatasetArchiveBudget(
		remoteDatasetArchiveExpandedLimitBytes,
		remoteDatasetArchiveMemberLimit,
		remoteDatasetArchiveDepthLimit,
	)
}

func (budget *remoteDatasetArchiveBudget) remainingBytes() int64 {
	if budget == nil || budget.maxBytes <= budget.usedBytes {
		return 0
	}
	return budget.maxBytes - budget.usedBytes
}

func (budget *remoteDatasetArchiveBudget) ensureBytes(size int64, description string) error {
	if budget == nil {
		return nil
	}
	if size < 0 || size > budget.remainingBytes() {
		return fmt.Errorf("%w: %s would exceed the %d byte expanded-data limit", errRemoteDatasetArchiveBudgetExceeded, description, budget.maxBytes)
	}
	return nil
}

func (budget *remoteDatasetArchiveBudget) consumeBytes(size int64, description string) error {
	if err := budget.ensureBytes(size, description); err != nil {
		return err
	}
	if budget != nil {
		budget.usedBytes += size
	}
	return nil
}

func (budget *remoteDatasetArchiveBudget) consumeMembers(count int, description string) error {
	if budget == nil {
		return nil
	}
	if count < 0 || count > budget.maxMembers-budget.usedMembers {
		return fmt.Errorf("%w: %s would exceed the %d member limit", errRemoteDatasetArchiveBudgetExceeded, description, budget.maxMembers)
	}
	budget.usedMembers += count
	return nil
}

func expandRemoteDatasetPayloads(data []byte, contentType, source string, depth int) ([]remoteDatasetPayload, error) {
	payloads, _, err := expandRemoteDatasetPayloadsWithWarnings(data, contentType, source, depth)
	return payloads, err
}

func expandRemoteDatasetPayloadsWithBudget(data []byte, contentType, source string, depth int, budget *remoteDatasetArchiveBudget) ([]remoteDatasetPayload, error) {
	payloads, _, err := expandRemoteDatasetPayloadsWithBudgetAndWarnings(data, contentType, source, depth, budget)
	return payloads, err
}

func expandRemoteDatasetPayloadsWithWarnings(data []byte, contentType, source string, depth int) ([]remoteDatasetPayload, []remoteDatasetParseWarning, error) {
	return expandRemoteDatasetPayloadsWithBudgetAndWarnings(data, contentType, source, depth, newDefaultRemoteDatasetArchiveBudget())
}

func expandRemoteDatasetPayloadsWithBudgetAndWarnings(data []byte, contentType, source string, depth int, budget *remoteDatasetArchiveBudget) ([]remoteDatasetPayload, []remoteDatasetParseWarning, error) {
	warnings := make([]remoteDatasetParseWarning, 0)
	payloads, err := expandRemoteDatasetPayloadsWithBudgetWarningSink(data, contentType, source, depth, budget, &warnings)
	return payloads, warnings, err
}

func expandRemoteDatasetPayloadsWithBudgetWarningSink(data []byte, contentType, source string, depth int, budget *remoteDatasetArchiveBudget, warnings *[]remoteDatasetParseWarning) ([]remoteDatasetPayload, error) {
	if budget == nil {
		budget = newDefaultRemoteDatasetArchiveBudget()
	}
	isZip := isZipPayload(data, contentType, source)
	isTar := isTarPayload(data, contentType, source)
	isGzip := isGzipPayload(data, contentType, source)
	isBzip2 := isBzip2Payload(data, contentType, source)
	isXz := isXzPayload(data, contentType, source)
	if (isZip || isTar || isGzip || isBzip2 || isXz) && budget != nil && depth >= budget.maxDepth {
		return nil, fmt.Errorf("%w: archive nesting exceeds %d layers at %q", errRemoteDatasetArchiveBudgetExceeded, budget.maxDepth, source)
	}
	if isZip {
		return expandZipRemoteDatasetPayloadWithBudgetWarningSink(data, source, depth, budget, warnings)
	}
	if isTar {
		return expandTarRemoteDatasetPayloadWithBudgetWarningSink(data, source, depth, budget, warnings)
	}
	if isGzip {
		decompressed, err := gunzipRemoteDatasetPayloadWithBudget(data, budget)
		if err != nil {
			return nil, err
		}
		return expandRemoteDatasetPayloadsWithBudgetWarningSink(decompressed, "", stripCompressionSuffix(source), depth+1, budget, warnings)
	}
	if isBzip2 {
		decompressed, err := bunzip2RemoteDatasetPayloadWithBudget(data, budget)
		if err != nil {
			return nil, err
		}
		return expandRemoteDatasetPayloadsWithBudgetWarningSink(decompressed, "", stripCompressionSuffix(source), depth+1, budget, warnings)
	}
	if isXz {
		decompressed, err := unxzRemoteDatasetPayloadWithBudget(data, budget)
		if err != nil {
			return nil, err
		}
		return expandRemoteDatasetPayloadsWithBudgetWarningSink(decompressed, "", stripCompressionSuffix(source), depth+1, budget, warnings)
	}
	return []remoteDatasetPayload{{Source: source, ContentType: contentType, Data: data}}, nil
}

func expandZipRemoteDatasetPayload(data []byte, source string, depth int) ([]remoteDatasetPayload, error) {
	return expandZipRemoteDatasetPayloadWithBudget(data, source, depth, newDefaultRemoteDatasetArchiveBudget())
}

func expandZipRemoteDatasetPayloadWithBudget(data []byte, source string, depth int, budget *remoteDatasetArchiveBudget) ([]remoteDatasetPayload, error) {
	return expandZipRemoteDatasetPayloadWithBudgetWarningSink(data, source, depth, budget, nil)
}

func expandZipRemoteDatasetPayloadWithBudgetWarningSink(data []byte, source string, depth int, budget *remoteDatasetArchiveBudget, warnings *[]remoteDatasetParseWarning) ([]remoteDatasetPayload, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if err := budget.consumeMembers(len(reader.File), fmt.Sprintf("zip archive %q", source)); err != nil {
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
		if file.UncompressedSize64 > uint64(remoteDatasetFetchLimitBytes) {
			return nil, fmt.Errorf("extracted file %q is larger than %d bytes", file.Name, remoteDatasetFetchLimitBytes)
		}
		if err := budget.ensureBytes(int64(file.UncompressedSize64), fmt.Sprintf("zip member %q", file.Name)); err != nil {
			return nil, err
		}
		nextSource := joinDatasetSource(source, file.Name)
		rc, err := file.Open()
		if err != nil {
			appendRemoteDatasetArchiveWarning(warnings, nextSource, "archive_member_open_failed", err)
			continue
		}
		fileData, readErr := readLimitedRemoteDatasetPayloadWithBudget(rc, fmt.Sprintf("extracted file %q", file.Name), budget)
		_ = rc.Close()
		if readErr != nil {
			if errors.Is(readErr, errRemoteDatasetArchiveBudgetExceeded) {
				return nil, readErr
			}
			appendRemoteDatasetArchiveWarning(warnings, nextSource, "archive_member_read_failed", readErr)
			continue
		}
		nested, err := expandRemoteDatasetPayloadsWithBudgetWarningSink(fileData, "", nextSource, depth+1, budget, warnings)
		if err != nil {
			if errors.Is(err, errRemoteDatasetArchiveBudgetExceeded) {
				return nil, err
			}
			appendRemoteDatasetArchiveWarning(warnings, nextSource, "nested_archive_decode_failed", err)
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
	return expandTarRemoteDatasetPayloadWithBudget(data, source, depth, newDefaultRemoteDatasetArchiveBudget())
}

func expandTarRemoteDatasetPayloadWithBudget(data []byte, source string, depth int, budget *remoteDatasetArchiveBudget) ([]remoteDatasetPayload, error) {
	return expandTarRemoteDatasetPayloadWithBudgetWarningSink(data, source, depth, budget, nil)
}

func expandTarRemoteDatasetPayloadWithBudgetWarningSink(data []byte, source string, depth int, budget *remoteDatasetArchiveBudget, warnings *[]remoteDatasetParseWarning) ([]remoteDatasetPayload, error) {
	reader := tar.NewReader(bytes.NewReader(data))
	payloads := make([]remoteDatasetPayload, 0)
	for {
		hdr, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if len(payloads) > 0 && warnings != nil {
				appendRemoteDatasetArchiveWarning(warnings, source, "archive_stream_read_failed", err)
				break
			}
			return nil, fmt.Errorf("read tar archive %q: %w", source, err)
		}
		if err := budget.consumeMembers(1, fmt.Sprintf("tar archive %q", source)); err != nil {
			return nil, err
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
		if err := budget.ensureBytes(hdr.Size, fmt.Sprintf("tar member %q", hdr.Name)); err != nil {
			return nil, err
		}
		nextSource := joinDatasetSource(source, hdr.Name)
		fileData, err := readLimitedRemoteDatasetPayloadWithBudget(reader, fmt.Sprintf("extracted file %q", hdr.Name), budget)
		if err != nil {
			if errors.Is(err, errRemoteDatasetArchiveBudgetExceeded) {
				return nil, err
			}
			appendRemoteDatasetArchiveWarning(warnings, nextSource, "archive_member_read_failed", err)
			break
		}
		nested, err := expandRemoteDatasetPayloadsWithBudgetWarningSink(fileData, "", nextSource, depth+1, budget, warnings)
		if err != nil {
			if errors.Is(err, errRemoteDatasetArchiveBudgetExceeded) {
				return nil, err
			}
			appendRemoteDatasetArchiveWarning(warnings, nextSource, "nested_archive_decode_failed", err)
			continue
		}
		payloads = append(payloads, nested...)
	}
	if len(payloads) == 0 {
		return nil, fmt.Errorf("tar archive %q did not contain any extractable dataset files", source)
	}
	return payloads, nil
}

func appendRemoteDatasetArchiveWarning(warnings *[]remoteDatasetParseWarning, source, reason string, err error) {
	if warnings == nil || err == nil {
		return
	}
	*warnings = append(*warnings, remoteDatasetParseWarning{
		Source: source,
		Reason: reason + ": " + err.Error(),
		Count:  1,
	})
}

func gunzipRemoteDatasetPayload(data []byte) ([]byte, error) {
	return gunzipRemoteDatasetPayloadWithBudget(data, nil)
}

func gunzipRemoteDatasetPayloadWithBudget(data []byte, budget *remoteDatasetArchiveBudget) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return readLimitedRemoteDatasetPayloadWithBudget(reader, "gzip payload", budget)
}

func bunzip2RemoteDatasetPayload(data []byte) ([]byte, error) {
	return bunzip2RemoteDatasetPayloadWithBudget(data, nil)
}

func bunzip2RemoteDatasetPayloadWithBudget(data []byte, budget *remoteDatasetArchiveBudget) ([]byte, error) {
	return readLimitedRemoteDatasetPayloadWithBudget(bzip2.NewReader(bytes.NewReader(data)), "bzip2 payload", budget)
}

func unxzRemoteDatasetPayload(data []byte) ([]byte, error) {
	return unxzRemoteDatasetPayloadWithBudget(data, nil)
}

func unxzRemoteDatasetPayloadWithBudget(data []byte, budget *remoteDatasetArchiveBudget) ([]byte, error) {
	reader, err := xz.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return readLimitedRemoteDatasetPayloadWithBudget(reader, "xz payload", budget)
}

func readLimitedRemoteDatasetPayload(reader io.Reader, description string) ([]byte, error) {
	return readLimitedRemoteDatasetPayloadWithBudget(reader, description, nil)
}

func readLimitedRemoteDatasetPayloadWithBudget(reader io.Reader, description string, budget *remoteDatasetArchiveBudget) ([]byte, error) {
	limit := int64(remoteDatasetFetchLimitBytes)
	if budget != nil && budget.remainingBytes() < limit {
		limit = budget.remainingBytes()
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if consumeErr := budget.consumeBytes(int64(len(data)), description); consumeErr != nil {
		return nil, consumeErr
	}
	if int64(len(data)) > limit {
		if limit < remoteDatasetFetchLimitBytes {
			return nil, fmt.Errorf("%w: %s would exceed the %d byte expanded-data limit", errRemoteDatasetArchiveBudgetExceeded, description, budget.maxBytes)
		}
		return nil, fmt.Errorf("%s is larger than %d bytes", description, remoteDatasetFetchLimitBytes)
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func isZipPayload(data []byte, contentType, source string) bool {
	if len(data) >= 4 && bytes.Equal(data[:4], []byte("PK\x03\x04")) {
		return true
	}
	ct := normalizedArchiveContentType(contentType)
	if ct == "application/zip" || ct == "application/x-zip" || ct == "application/x-zip-compressed" || strings.HasSuffix(ct, "+zip") {
		return true
	}
	lower := strings.ToLower(source)
	return strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".jar") || strings.HasSuffix(lower, ".war")
}

func isTarPayload(data []byte, contentType, source string) bool {
	if len(data) >= 262 && bytes.Equal(data[257:262], []byte("ustar")) {
		return true
	}
	ct := normalizedArchiveContentType(contentType)
	if ct == "application/tar" || ct == "application/x-tar" {
		return true
	}
	lower := strings.ToLower(source)
	return strings.HasSuffix(lower, ".tar")
}

func isGzipPayload(data []byte, contentType, source string) bool {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		return true
	}
	ct := normalizedArchiveContentType(contentType)
	if ct == "application/gzip" || ct == "application/x-gzip" {
		return true
	}
	lower := strings.ToLower(source)
	return strings.HasSuffix(lower, ".gz") || strings.HasSuffix(lower, ".tgz") || strings.HasSuffix(lower, ".tar.gz")
}

func isBzip2Payload(data []byte, contentType, source string) bool {
	if len(data) >= 3 && bytes.Equal(data[:3], []byte("BZh")) {
		return true
	}
	ct := normalizedArchiveContentType(contentType)
	if ct == "application/bzip2" || ct == "application/x-bzip2" {
		return true
	}
	lower := strings.ToLower(source)
	return strings.HasSuffix(lower, ".bz2") || strings.HasSuffix(lower, ".tbz2") || strings.HasSuffix(lower, ".tbz")
}

func isXzPayload(data []byte, contentType, source string) bool {
	if len(data) >= 6 && bytes.Equal(data[:6], []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}) {
		return true
	}
	ct := normalizedArchiveContentType(contentType)
	if ct == "application/x-xz" || ct == "application/xz" {
		return true
	}
	lower := strings.ToLower(source)
	return strings.HasSuffix(lower, ".xz") || strings.HasSuffix(lower, ".txz")
}

func normalizedArchiveContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if semicolon := strings.IndexByte(contentType, ';'); semicolon >= 0 {
		contentType = strings.TrimSpace(contentType[:semicolon])
	}
	return contentType
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
		strings.HasSuffix(base, ".class"):
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
