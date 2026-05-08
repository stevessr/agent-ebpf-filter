package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ulikunitz/xz"
)

const remoteDatasetFetchLimitBytes = 20 << 20
const remoteDatasetUploadLimitBytes = 100 << 20

type remoteDatasetRequest struct {
	URL            string `json:"url"`
	Content        string `json:"content"`
	ContentBase64  string `json:"contentBase64"`
	SourceName     string `json:"sourceName"`
	Format         string `json:"format"`
	Limit          int    `json:"limit"`
	LabelMode      string `json:"labelMode"`
	ImportAll      bool   `json:"importAll"`
	CleanSensitive bool   `json:"cleanSensitive"`
}

type remoteDatasetRow struct {
	Row          int      `json:"row"`
	Source       string   `json:"source,omitempty"`
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
	Source      string
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

		sample := buildRemoteDatasetSample(row, req.LabelMode, req.CleanSensitive)
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
		payloadRecords, payloadFormat, parseErr := parseRemoteDatasetRecords(payload.Data, req.Format, payload.Source)
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
		row := buildRemoteDatasetRow(record, req.LabelMode, req.CleanSensitive)
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
	case "block", "dangerous", "highrisk", "high-risk":
		return "block"
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
		CommandLine:  trainingSampleCommandLine(sample),
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
