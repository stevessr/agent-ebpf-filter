package main

import (
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
)

const remoteDatasetFetchLimitBytes = 20 << 20

type remoteDatasetRequest struct {
	URL       string `json:"url"`
	Format    string `json:"format"`
	Limit     int    `json:"limit"`
	LabelMode string `json:"labelMode"`
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

func bindRemoteDatasetRequest(c *gin.Context) (remoteDatasetRequest, bool) {
	var req remoteDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return req, false
	}

	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return req, false
	}
	if _, err := validateRemoteDatasetURL(req.URL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return req, false
	}

	req.Format = normalizeRemoteDatasetFormat(req.Format, req.URL)
	req.Limit = parseDatasetLimit(req.Limit)
	req.LabelMode = normalizeRemoteDatasetLabelMode(req.LabelMode)
	return req, true
}

func pullRemoteDataset(req remoteDatasetRequest) (*remoteDatasetResponse, error) {
	downloaded, contentType, err := downloadRemoteDataset(req.URL)
	if err != nil {
		return nil, err
	}

	records, format, err := parseRemoteDatasetRecords(downloaded, req.Format)
	if err != nil {
		return nil, err
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
	if req.Limit > 0 && len(rows) > req.Limit {
		rows = rows[:req.Limit]
		truncated = true
	}

	return &remoteDatasetResponse{
		Source:      req.URL,
		Format:      format,
		ContentType: contentType,
		Total:       len(records),
		Limit:       req.Limit,
		Truncated:   truncated,
		Rows:        rows,
	}, nil
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

func parseDatasetLimit(limit int) int {
	if limit <= 0 {
		return 200
	}
	if limit > 5000 {
		return 5000
	}
	return limit
}

func parseRemoteDatasetRecords(raw []byte, format string) ([]remoteDatasetRecord, string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "auto"
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
		record.UserLabel = "remote-import"
		records = append(records, record)
	}
	return records
}

func flattenDatasetJSON(decoded any) []any {
	switch value := decoded.(type) {
	case []any:
		return value
	case map[string]any:
		for _, key := range []string{"rows", "records", "items", "samples", "data", "commands"} {
			if nested, ok := value[key]; ok {
				if arr, ok := nested.([]any); ok {
					return arr
				}
			}
		}
		return []any{value}
	default:
		return []any{decoded}
	}
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

	commandLine := firstStringValue(row, "commandLine", "cmdline", "full_command", "command", "shell", "text")
	comm := firstStringValue(row, "comm", "commandName", "name", "executable")
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

	record.Category = firstStringValue(row, "category", "behavior", "type", "group")
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
			assessment := assessCommandSafety(comm, args, "", 0)
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
