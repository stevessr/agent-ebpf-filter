package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ulikunitz/xz"
)

const remoteDatasetFetchLimitBytes = 20 << 20
const remoteDatasetUploadLimitBytes = 100 << 20

type remoteDatasetRequest struct {
	URL           string `json:"url"`
	Content       string `json:"content"`
	ContentBase64 string `json:"contentBase64"`
	SourceName    string `json:"sourceName"`
	Format        string `json:"format"`
	Limit         int    `json:"limit"`
	LabelMode     string `json:"labelMode"`
	ImportAll     bool   `json:"importAll"`
}

type remoteDatasetRow struct {
	Row          int      `json:"row"`
	CommandLine  string   `json:"commandLine"`
	Comm         string   `json:"comm"`
	Args         []string `json:"args"`
	Label        string   `json:"label"`
	LabelSource  string   `json:"labelSource"`
	Category     string   `json:"category"`
	AnomalyScore float64  `json:"anomalyScore"`
	HasAnomaly   bool     `json:"-"`
	Timestamp    string   `json:"timestamp"`
	UserLabel    string   `json:"userLabel"`
	Duplicate    bool     `json:"duplicate"`
}

type remoteDatasetResponse struct {
	Source         string             `json:"source"`
	Format         string             `json:"format"`
	ContentType    string             `json:"contentType"`
	Total          int                `json:"total"`
	Limit          int                `json:"limit"`
	Truncated      bool               `json:"truncated"`
	Imported       int                `json:"imported,omitempty"`
	Skipped        int                `json:"skipped,omitempty"`
	TotalSamples   int                `json:"totalSamples,omitempty"`
	LabeledSamples int                `json:"labeledSamples,omitempty"`
	Rows           []remoteDatasetRow `json:"rows,omitempty"`
}

type remoteDatasetRecord struct {
	Row         int
	CommandLine string
	Comm        string
	Args        []string
	Label       string
	LabelSource string
	Category    string
	Anomaly     float64
	HasAnomaly  bool
	Timestamp   time.Time
	UserLabel   string
}

type remoteDatasetPayload struct {
	Source      string
	ContentType string
	Data        []byte
}

func handleMLDatasetPullPost(c *gin.Context) {
	req, ok := bindRemoteDatasetRequest(c)
	if !ok {
		return
	}

	resp, err := pullRemoteDataset(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func handleMLDatasetImportPost(c *gin.Context) {
	req, ok := bindRemoteDatasetRequest(c)
	if !ok {
		return
	}
	if globalTrainingStore == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ML training store not initialized"})
		return
	}

	resp, err := pullRemoteDataset(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	imported := 0
	skipped := 0
	seen := make(map[string]struct{})
	for _, row := range resp.Rows {
		if row.Comm == "" {
			skipped++
			continue
		}
		key := commandKey(row.Comm, row.Args)
		if _, exists := seen[key]; exists {
			skipped++
			continue
		}
		seen[key] = struct{}{}
		if globalTrainingStore.HasExactCommand(row.Comm, row.Args) {
			skipped++
			continue
		}

		sample := buildRemoteDatasetSample(row, req.LabelMode)
		globalTrainingStore.Add(sample)
		recordCommandSampleSideEffects(sample)
		imported++
	}

	if err := globalTrainingStore.Flush(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "imported remote dataset but failed to persist: " + err.Error()})
		return
	}

	total, labeled := globalTrainingStore.Status()
	resp.Imported = imported
	resp.Skipped = skipped
	resp.TotalSamples = total
	resp.LabeledSamples = labeled
	c.JSON(http.StatusOK, resp)
}

func handleMLDatasetExportGet(c *gin.Context) {
	if globalTrainingStore == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ML training store not initialized"})
		return
	}

	items := globalTrainingStore.AllSamplesWithIndex()
	rows := make([]remoteDatasetRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, trainingSampleToRemoteDatasetRow(item.Index, item.Sample))
	}
	total, labeled := globalTrainingStore.Status()
	resp := remoteDatasetResponse{
		Source:         "local-training-store",
		Format:         "json",
		ContentType:    "application/json",
		Total:          total,
		Limit:          total,
		Truncated:      false,
		TotalSamples:   total,
		LabeledSamples: labeled,
		Rows:           rows,
	}
	c.Header("Content-Disposition", `attachment; filename="agent-ebpf-filter-training-dataset.json"`)
	c.JSON(http.StatusOK, resp)
}

func handleMLDatasetClearDelete(c *gin.Context) {
	if globalTrainingStore == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ML training store not initialized"})
		return
	}

	cleared := globalTrainingStore.Clear()
	if err := globalTrainingStore.Flush(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cleared training store but failed to persist: " + err.Error()})
		return
	}

	total, labeled := globalTrainingStore.Status()
	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"cleared":        cleared,
		"totalSamples":   total,
		"labeledSamples": labeled,
	})
}

func bindRemoteDatasetRequest(c *gin.Context) (remoteDatasetRequest, bool) {
	var req remoteDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return req, false
	}

	req.URL = strings.TrimSpace(req.URL)
	req.SourceName = strings.TrimSpace(req.SourceName)
	req.ContentBase64 = strings.TrimSpace(req.ContentBase64)
	hasContent := strings.TrimSpace(req.Content) != "" || req.ContentBase64 != ""
	if req.URL == "" && !hasContent {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url or content is required"})
		return req, false
	}
	if req.URL != "" {
		if _, err := validateRemoteDatasetURL(req.URL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return req, false
		}
	}

	sourceRef := req.SourceName
	if sourceRef == "" {
		sourceRef = req.URL
	}
	if hasContent {
		req.Format = strings.ToLower(strings.TrimSpace(req.Format))
		if req.Format != "" && req.Format != "auto" {
			req.Format = normalizeRemoteDatasetFormat(req.Format, "")
		} else {
			req.Format = "auto"
		}
	} else {
		req.Format = normalizeRemoteDatasetFormat(req.Format, sourceRef)
	}
	req.Limit = parseDatasetLimit(req.Limit)
	req.LabelMode = normalizeRemoteDatasetLabelMode(req.LabelMode)
	return req, true
}

func pullRemoteDataset(req remoteDatasetRequest) (*remoteDatasetResponse, error) {
	downloaded, contentType, source, err := loadRemoteDatasetPayload(req)
	if err != nil {
		return nil, err
	}
	if looksLikeHTMLDataset(downloaded, contentType) {
		if source == "" {
			source = req.URL
		}
		return nil, fmt.Errorf("dataset source %q looks like an HTML landing page; please use a raw file URL or import a local file instead", source)
	}

	payloads, err := expandRemoteDatasetPayloads(downloaded, contentType, source, 0)
	if err != nil {
		return nil, err
	}

	records := make([]remoteDatasetRecord, 0)
	format := ""
	for _, payload := range payloads {
		payloadRecords, payloadFormat, parseErr := parseRemoteDatasetRecords(payload.Data, req.Format)
		if parseErr != nil {
			if len(payloads) == 1 {
				return nil, parseErr
			}
			continue
		}
		if len(payloadRecords) == 0 {
			continue
		}
		records = append(records, payloadRecords...)
		format = mergeDatasetFormat(format, payloadFormat)
	}
	if len(records) == 0 {
		return nil, errors.New("no dataset records found in payload")
	}
	if format == "" {
		format = normalizeRemoteDatasetFormat(req.Format, source)
	}

	rows := make([]remoteDatasetRow, 0, len(records))
	for _, record := range records {
		row := buildRemoteDatasetRow(record)
		if globalTrainingStore != nil {
			row.Duplicate = globalTrainingStore.HasExactCommand(row.Comm, row.Args)
		}
		rows = append(rows, row)
	}

	truncated := false
	if !req.ImportAll && req.Limit > 0 && len(rows) > req.Limit {
		rows = rows[:req.Limit]
		truncated = true
	}
	limit := req.Limit
	if req.ImportAll {
		limit = len(rows)
	}
	if contentType == "" {
		contentType = contentTypeForDatasetFormat(format)
	}
	if source == "" {
		source = req.URL
	}

	return &remoteDatasetResponse{
		Source:      source,
		Format:      format,
		ContentType: contentType,
		Total:       len(records),
		Limit:       limit,
		Truncated:   truncated,
		Rows:        rows,
	}, nil
}

func loadRemoteDatasetPayload(req remoteDatasetRequest) ([]byte, string, string, error) {
	if strings.TrimSpace(req.ContentBase64) != "" {
		raw, err := base64.StdEncoding.DecodeString(req.ContentBase64)
		if err != nil {
			return nil, "", "", fmt.Errorf("invalid base64 dataset content: %w", err)
		}
		if len(raw) > remoteDatasetUploadLimitBytes {
			return nil, "", "", fmt.Errorf("remote dataset content is larger than %d bytes", remoteDatasetUploadLimitBytes)
		}
		source := strings.TrimSpace(req.SourceName)
		if source == "" {
			source = "inline"
		}
		return raw, "", source, nil
	}
	if strings.TrimSpace(req.Content) != "" {
		raw := []byte(req.Content)
		if len(raw) > remoteDatasetUploadLimitBytes {
			return nil, "", "", fmt.Errorf("remote dataset content is larger than %d bytes", remoteDatasetUploadLimitBytes)
		}
		source := strings.TrimSpace(req.SourceName)
		if source == "" {
			source = "inline"
		}
		return raw, "", source, nil
	}
	downloaded, contentType, err := downloadRemoteDataset(req.URL)
	if err != nil {
		return nil, "", "", err
	}
	source := req.URL
	if req.SourceName != "" {
		source = req.SourceName
	}
	return downloaded, contentType, source, nil
}

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

func downloadRemoteDataset(rawURL string) ([]byte, string, error) {
	parsed, err := validateRemoteDatasetURL(rawURL)
	if err != nil {
		return nil, "", err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "agent-ebpf-filter/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("remote dataset fetch failed: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, remoteDatasetFetchLimitBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > remoteDatasetFetchLimitBytes {
		return nil, "", fmt.Errorf("remote dataset is larger than %d bytes", remoteDatasetFetchLimitBytes)
	}

	contentType := resp.Header.Get("Content-Type")
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
		contentType = mediaType
	}
	return body, contentType, nil
}

func validateRemoteDatasetURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported dataset URL scheme: %s", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, errors.New("dataset URL must include a host")
	}
	return parsed, nil
}

func normalizeRemoteDatasetFormat(format, sourceURL string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format != "" && format != "auto" {
		return format
	}

	ext := strings.ToLower(filepath.Ext(sourceURL))
	switch ext {
	case ".json":
		return "json"
	case ".jsonl", ".ndjson":
		return "jsonl"
	case ".csv":
		return "csv"
	case ".tsv":
		return "tsv"
	case ".txt", ".log":
		return "text"
	default:
		return "auto"
	}
}

func normalizeRemoteDatasetLabelMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "preserve", "keep", "source":
		return "preserve"
	case "unlabeled", "manual", "none":
		return "unlabeled"
	case "heuristic", "auto", "automatic":
		return "heuristic"
	default:
		return "preserve"
	}
}

func contentTypeForDatasetFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return "application/json"
	case "jsonl", "ndjson":
		return "application/x-ndjson"
	case "csv":
		return "text/csv; charset=utf-8"
	case "tsv":
		return "text/tab-separated-values; charset=utf-8"
	case "text", "txt":
		return "text/plain; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

func parseDatasetLimit(limit int) int {
	if limit <= 0 {
		return 200
	}
	if limit > 5000 {
		return 5000
	}
	return limit
}

func trainingSampleToRemoteDatasetRow(index int, sample TrainingSample) remoteDatasetRow {
	label := sampleLabelName(sample.Label)
	if label == "" {
		label = "-"
	}
	timestamp := sample.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	return remoteDatasetRow{
		Row:          index,
		CommandLine:  joinCommandLine(sample.Comm, sample.Args),
		Comm:         sample.Comm,
		Args:         append([]string(nil), sample.Args...),
		Label:        label,
		LabelSource:  sample.UserLabel,
		Category:     sample.Category,
		AnomalyScore: sample.AnomalyScore,
		HasAnomaly:   true,
		Timestamp:    timestamp.Format(time.RFC3339),
		UserLabel:    sample.UserLabel,
	}
}

func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// Check first 1024 bytes for null bytes or excessive non-printable characters
	checkLen := len(data)
	if checkLen > 1024 {
		checkLen = 1024
	}
	nullCount := 0
	controlCount := 0
	for i := 0; i < checkLen; i++ {
		b := data[i]
		if b == 0 {
			nullCount++
		} else if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			controlCount++
		}
	}
	// Binary files almost always have nulls or many control characters.
	// ASCII/UTF-8 text files should not have nulls and very few control characters.
	return nullCount > 0 || controlCount > (checkLen/10)
}

func parseRemoteDatasetRecords(raw []byte, format string) ([]remoteDatasetRecord, string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "auto"
	}

	// Early check for binary data if format is auto or text
	if (format == "auto" || format == "text" || format == "txt") && isBinary(raw) {
		// If it's binary but we're here, it means it wasn't recognized as an archive
		// or it's a corrupted archive. We should NOT treat it as text.
		return nil, "", errors.New("unsupported binary data format; expected JSON, CSV, TSV or plain text")
	}

	switch format {
	case "json":
		return parseJSONDatasetRecords(raw)
	case "jsonl", "ndjson":
		return parseJSONLinesDatasetRecords(raw)
	case "csv":
		return parseDelimitedDatasetRecords(raw, ',')
	case "tsv":
		return parseDelimitedDatasetRecords(raw, '\t')
	case "text", "txt":
		return parseTextDatasetRecords(raw), "text", nil
	case "auto":
		if looksLikeJSON(raw) {
			if records, detected, err := parseJSONDatasetRecords(raw); err == nil {
				return records, detected, nil
			}
			if records, detected, err := parseJSONLinesDatasetRecords(raw); err == nil && len(records) > 0 {
				return records, detected, nil
			}
		}
		if looksLikeDelimited(raw) {
			if records, detected, err := parseDelimitedDatasetRecords(raw, ','); err == nil && len(records) > 0 {
				return records, detected, nil
			}
			if records, detected, err := parseDelimitedDatasetRecords(raw, '\t'); err == nil && len(records) > 0 {
				return records, detected, nil
			}
		}
		return parseTextDatasetRecords(raw), "text", nil
	default:
		return nil, "", fmt.Errorf("unsupported dataset format %q", format)
	}
}

func looksLikeJSON(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func looksLikeDelimited(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return false
	}
	firstLine := trimmed
	if idx := strings.IndexByte(trimmed, '\n'); idx >= 0 {
		firstLine = trimmed[:idx]
	}
	return strings.Contains(firstLine, ",") || strings.Contains(firstLine, "\t")
}

func parseJSONDatasetRecords(raw []byte) ([]remoteDatasetRecord, string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, "json", nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, "", err
	}

	items := flattenDatasetJSON(decoded)
	if len(items) == 0 {
		return nil, "json", nil
	}
	records := make([]remoteDatasetRecord, 0, len(items))
	for i, item := range items {
		record, ok := remoteDatasetRecordFromAny(item, i+1)
		if !ok {
			continue
		}
		records = append(records, record)
	}
	return records, "json", nil
}

func parseJSONLinesDatasetRecords(raw []byte) ([]remoteDatasetRecord, string, error) {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	records := make([]remoteDatasetRecord, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var decoded any
		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&decoded); err != nil {
			continue
		}
		record, ok := remoteDatasetRecordFromAny(decoded, i+1)
		if !ok {
			continue
		}
		records = append(records, record)
	}
	return records, "jsonl", nil
}

func parseDelimitedDatasetRecords(raw []byte, comma rune) ([]remoteDatasetRecord, string, error) {
	reader := csv.NewReader(strings.NewReader(string(raw)))
	reader.Comma = comma
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, "", err
	}
	if len(rows) == 0 {
		return nil, "", nil
	}

	header := normalizeHeaderRow(rows[0])
	if len(header) == 0 {
		header = make([]string, len(rows[0]))
		for i := range header {
			header[i] = fmt.Sprintf("column_%d", i)
		}
	}

	records := make([]remoteDatasetRecord, 0, len(rows)-1)
	for i := 1; i < len(rows); i++ {
		rowMap := make(map[string]any, len(header))
		for j, cell := range rows[i] {
			if j < len(header) {
				rowMap[header[j]] = strings.TrimSpace(cell)
			}
		}
		record, ok := remoteDatasetRecordFromMap(rowMap, i+1)
		if !ok {
			continue
		}
		records = append(records, record)
	}

	format := "csv"
	if comma == '\t' {
		format = "tsv"
	}
	return records, format, nil
}

func parseTextDatasetRecords(raw []byte) []remoteDatasetRecord {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	records := make([]remoteDatasetRecord, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		record := remoteDatasetRecord{Row: i + 1}
		record.CommandLine = line
		record.Comm, record.Args = normalizeCommandInput(line, "", nil)
		if record.Comm == "" {
			continue
		}
		// Skip pure integers (likely syscall traces from datasets like ADFA-LD)
		if _, err := strconv.Atoi(record.Comm); err == nil {
			continue
		}
		record.UserLabel = "remote-import"
		records = append(records, record)
	}
	return records
}

func flattenDatasetJSON(decoded any) []any {
	var items []any
	switch value := decoded.(type) {
	case []any:
		items = value
	case map[string]any:
		found := false
		for _, key := range []string{"rows", "records", "items", "samples", "data", "commands"} {
			if nested, ok := value[key]; ok {
				if arr, ok := nested.([]any); ok {
					items = arr
					found = true
					break
				}
			}
		}
		if !found {
			// Check if it's a map of objects (GTFOBins style)
			allObjects := true
			for _, v := range value {
				if _, ok := v.(map[string]any); !ok {
					allObjects = false
					break
				}
			}
			if allObjects && len(value) > 0 {
				for k, v := range value {
					m := v.(map[string]any)
					m["_injected_name"] = k
					items = append(items, m)
				}
			} else {
				items = []any{value}
			}
		}
	default:
		return []any{decoded}
	}

	// Second pass: expand nested commands (GTFOBins 'functions' or LOLBAS 'Commands')
	var expanded []any
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			expanded = append(expanded, item)
			continue
		}

		// GTFOBins expansion
		if funcs, ok := m["functions"].(map[string]any); ok {
			for fName, fList := range funcs {
				if fl, ok := fList.([]any); ok {
					for _, fi := range fl {
						if fim, ok := fi.(map[string]any); ok {
							newM := make(map[string]any)
							for k, v := range m { // copy original
								if k != "functions" {
									newM[k] = v
								}
							}
							for k, v := range fim { // merge function entry
								newM[k] = v
							}
							newM["_injected_category"] = fName
							expanded = append(expanded, newM)
						}
					}
				}
			}
			continue
		}

		// LOLBAS expansion
		if cmds, ok := m["Commands"].([]any); ok {
			for _, ci := range cmds {
				if cim, ok := ci.(map[string]any); ok {
					newM := make(map[string]any)
					for k, v := range m { // copy original
						if k != "Commands" {
							newM[k] = v
						}
					}
					for k, v := range cim { // merge command entry
						newM[k] = v
					}
					expanded = append(expanded, newM)
				}
			}
			continue
		}

		expanded = append(expanded, m)
	}

	return expanded
}

func remoteDatasetRecordFromAny(decoded any, rowIndex int) (remoteDatasetRecord, bool) {
	switch value := decoded.(type) {
	case string:
		comm, args := normalizeCommandInput(value, "", nil)
		if comm == "" {
			return remoteDatasetRecord{}, false
		}
		return remoteDatasetRecord{
			Row:         rowIndex,
			CommandLine: value,
			Comm:        comm,
			Args:        args,
			UserLabel:   "remote-import",
		}, true
	case map[string]any:
		return remoteDatasetRecordFromMap(value, rowIndex)
	default:
		return remoteDatasetRecord{}, false
	}
}

func remoteDatasetRecordFromMap(row map[string]any, rowIndex int) (remoteDatasetRecord, bool) {
	record := remoteDatasetRecord{Row: rowIndex, UserLabel: "remote-import"}

	commandLine := firstStringValue(row, "commandLine", "cmdline", "full_command", "command", "shell", "text", "Command", "code")
	comm := firstStringValue(row, "comm", "commandName", "name", "executable", "Name", "_injected_name")
	args := extractDatasetArgs(row, commandLine)
	if commandLine == "" && comm != "" {
		commandLine = joinCommandLine(comm, args)
	}
	if commandLine != "" && comm == "" {
		comm, args = normalizeCommandInput(commandLine, "", nil)
	}
	if comm == "" && commandLine == "" {
		return remoteDatasetRecord{}, false
	}

	record.CommandLine = commandLine
	record.Comm = comm
	record.Args = args
	record.Label = normalizeDatasetLabelValue(row["label"])
	if record.Label == "" {
		record.Label = normalizeDatasetLabelValue(row["action"])
	}
	if record.Label == "" {
		record.Label = normalizeDatasetLabelValue(row["class"])
	}
	if record.Label != "" {
		record.LabelSource = "dataset"
	}

	record.Category = firstStringValue(row, "category", "behavior", "type", "group", "Category", "_injected_category")
	if anomaly, ok := extractDatasetFloat(row, "anomalyScore", "anomaly_score", "score", "riskScore"); ok {
		record.Anomaly = anomaly
		record.HasAnomaly = true
	}
	if ts, ok := extractDatasetTimestamp(row); ok {
		record.Timestamp = ts
	}
	if userLabel := firstStringValue(row, "userLabel", "user_label"); userLabel != "" {
		record.UserLabel = userLabel
	}

	return record, true
}

func buildRemoteDatasetRow(record remoteDatasetRecord) remoteDatasetRow {
	comm, args := normalizeCommandInput(record.CommandLine, record.Comm, record.Args)
	label := record.Label
	labelSource := record.LabelSource
	if label == "" {
		label = "-"
	}
	if labelSource == "" {
		labelSource = "inferred"
	}

	category := record.Category
	if category == "" {
		category = ClassifyBehavior(comm, args).PrimaryCategory
	}
	anomaly := record.Anomaly
	if !record.HasAnomaly {
		_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
		anomaly = globalEmbedder.ComputeAnomalyScore(emb)
	}

	timestamp := record.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	return remoteDatasetRow{
		Row:          record.Row,
		CommandLine:  joinCommandLine(comm, args),
		Comm:         comm,
		Args:         args,
		Label:        label,
		LabelSource:  labelSource,
		Category:     category,
		AnomalyScore: anomaly,
		HasAnomaly:   record.HasAnomaly,
		Timestamp:    timestamp.Format(time.RFC3339),
		UserLabel:    record.UserLabel,
	}
}

func buildRemoteDatasetSample(row remoteDatasetRow, mode string) TrainingSample {
	comm, args := normalizeCommandInput(row.CommandLine, row.Comm, row.Args)
	timestamp := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, row.Timestamp); err == nil {
		timestamp = parsed.UTC()
	}

	category := row.Category
	if category == "" {
		category = ClassifyBehavior(comm, args).PrimaryCategory
	}
	anomaly := row.AnomalyScore
	if !row.HasAnomaly {
		_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
		anomaly = globalEmbedder.ComputeAnomalyScore(emb)
	}

	label := int32(-1)
	userLabel := "remote-import"
	if mode == "unlabeled" {
		userLabel = "remote-import-unlabeled"
	} else {
		if normalized := normalizeActionLabel(row.Label); normalized != "" {
			label = actionFromLabel(normalized)
			userLabel = "remote-source-label"
		} else if mode == "heuristic" {
			assessment := assessCommandSafety(context.Background(), comm, args, "", 0)
			if action, ok := assessment["recommendedAction"].(string); ok {
				label = actionFromLabel(action)
				userLabel = "remote-heuristic"
			}
		}
	}

	features := globalFeatureExtractor.Extract(comm, args, "", 0)
	return TrainingSample{
		Features:     features,
		Label:        label,
		Comm:         comm,
		Args:         args,
		Category:     category,
		AnomalyScore: anomaly,
		Timestamp:    timestamp,
		UserLabel:    userLabel,
	}
}

func normalizeDatasetLabelValue(raw any) string {
	switch v := raw.(type) {
	case string:
		return normalizeActionLabel(v)
	case json.Number:
		if n, err := strconv.Atoi(v.String()); err == nil {
			return actionLabel[int32(n)]
		}
	case float64:
		return actionLabel[int32(v)]
	case int:
		return actionLabel[int32(v)]
	case int64:
		return actionLabel[int32(v)]
	case uint32:
		return actionLabel[int32(v)]
	case uint64:
		return actionLabel[int32(v)]
	}
	return ""
}

func normalizeActionLabel(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "0", "ALLOW", "BENIGN", "SAFE", "NORMAL", "PASSED", "PASS":
		return "ALLOW"
	case "1", "BLOCK", "DENY", "REJECT", "MALICIOUS", "BAD":
		return "BLOCK"
	case "2", "REWRITE", "TRANSFORM", "MODIFY":
		return "REWRITE"
	case "3", "ALERT", "WARN", "WARNING", "SUSPICIOUS":
		return "ALERT"
	default:
		return ""
	}
}

func extractDatasetArgs(row map[string]any, commandLine string) []string {
	if args := extractDatasetStringSlice(row, "args", "argv", "arguments", "commandArgs"); len(args) > 0 {
		return args
	}
	if raw := firstAnyValue(row, "args", "argv", "arguments", "commandArgs"); raw != nil {
		if str, ok := raw.(string); ok && strings.TrimSpace(str) != "" {
			return splitCommandLine(str)
		}
	}
	if commandLine != "" {
		_, args := normalizeCommandInput(commandLine, "", nil)
		return args
	}
	return nil
}

func extractDatasetStringSlice(row map[string]any, keys ...string) []string {
	for _, key := range keys {
		raw, ok := row[key]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
		case []any:
			out := make([]string, 0, len(value))
			for _, item := range value {
				if s := fmt.Sprint(item); strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
				}
			}
			if len(out) > 0 {
				return out
			}
		case string:
			if strings.TrimSpace(value) != "" {
				return splitCommandLine(value)
			}
		}
	}
	return nil
}

func extractDatasetFloat(row map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		raw, ok := row[key]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
		case float64:
			return value, true
		case float32:
			return float64(value), true
		case int:
			return float64(value), true
		case int64:
			return float64(value), true
		case json.Number:
			if f, err := value.Float64(); err == nil {
				return f, true
			}
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

func extractDatasetTimestamp(row map[string]any) (time.Time, bool) {
	raw := firstAnyValue(row, "timestamp", "time", "createdAt", "created_at", "ts")
	if raw == nil {
		return time.Time{}, false
	}
	switch value := raw.(type) {
	case string:
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		} {
			if ts, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
				return ts, true
			}
		}
		if num, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			return parseUnixTimestamp(num), true
		}
	case float64:
		return parseUnixTimestamp(int64(value)), true
	case json.Number:
		if n, err := value.Int64(); err == nil {
			return parseUnixTimestamp(n), true
		}
	case int64:
		return parseUnixTimestamp(value), true
	case int:
		return parseUnixTimestamp(int64(value)), true
	}
	return time.Time{}, false
}

func parseUnixTimestamp(v int64) time.Time {
	switch {
	case v > 1_000_000_000_000:
		return time.Unix(0, v*int64(time.Millisecond)).UTC()
	case v > 1_000_000_000:
		return time.Unix(v, 0).UTC()
	default:
		return time.Unix(v, 0).UTC()
	}
}

func firstStringValue(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := row[key]; ok {
			if s := fmt.Sprint(raw); strings.TrimSpace(s) != "" && s != "<nil>" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func firstAnyValue(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if raw, ok := row[key]; ok {
			if raw != nil {
				return raw
			}
		}
	}
	return nil
}

func normalizeHeaderRow(headers []string) []string {
	out := make([]string, 0, len(headers))
	for _, header := range headers {
		header = strings.ToLower(strings.TrimSpace(header))
		header = strings.ReplaceAll(header, " ", "")
		header = strings.ReplaceAll(header, "-", "_")
		if header != "" {
			out = append(out, header)
		} else {
			out = append(out, "")
		}
	}
	return out
}
