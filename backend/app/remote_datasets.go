package app

import (
	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/internal/behavior"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section remote_datasets.go ----

const remoteDatasetFetchLimitBytes = 20 << 20
const remoteDatasetUploadLimitBytes = 100 << 20
const remoteDatasetRequestBodyLimitBytes = 160 << 20

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
	Source            string                        `json:"source"`
	Format            string                        `json:"format"`
	ContentType       string                        `json:"contentType"`
	Total             int                           `json:"total"`
	TotalIsLowerBound bool                          `json:"totalIsLowerBound,omitempty"`
	Limit             int                           `json:"limit"`
	RecordLimit       int                           `json:"recordLimit,omitempty"`
	Truncated         bool                          `json:"truncated"`
	Imported          int                           `json:"imported,omitempty"`
	Skipped           int                           `json:"skipped,omitempty"`
	TotalSamples      int                           `json:"totalSamples,omitempty"`
	LabeledSamples    int                           `json:"labeledSamples,omitempty"`
	ByLabel           []researchCount               `json:"byLabel,omitempty"`
	ByCategory        []researchCount               `json:"byCategory,omitempty"`
	BySource          []researchCount               `json:"bySource,omitempty"`
	SkipReasons       []researchCount               `json:"skipReasons,omitempty"`
	ParseWarnings     []remoteDatasetParseWarning   `json:"parseWarnings,omitempty"`
	Normalization     ml.FeatureNormalizationReport `json:"normalization,omitempty"`
	Quality           DatasetQualitySummary         `json:"quality,omitempty"`
	Rows              []remoteDatasetRow            `json:"rows,omitempty"`
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
	if ml.GlobalTrainingStore == nil {
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
	skipReasons := map[string]int{}
	for _, row := range resp.Rows {
		if row.Comm == "" {
			skipped++
			incrementResearchCount(skipReasons, "empty_comm")
			continue
		}
		key := behavior.CommandKey(row.Comm, row.Args)
		if _, exists := seen[key]; exists {
			skipped++
			incrementResearchCount(skipReasons, "duplicate_in_payload")
			continue
		}
		seen[key] = struct{}{}
		if ml.GlobalTrainingStore.HasExactCommand(row.Comm, row.Args) {
			skipped++
			incrementResearchCount(skipReasons, "duplicate_in_store")
			continue
		}

		sample := buildRemoteDatasetSample(row, req.LabelMode, req.CleanSensitive)
		ml.GlobalTrainingStore.Add(sample)
		recordCommandSampleSideEffects(sample)
		imported++
	}

	if err := ml.GlobalTrainingStore.Flush(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "imported remote dataset but failed to persist: " + err.Error()})
		return
	}

	total, labeled := ml.GlobalTrainingStore.Status()
	resp.Imported = imported
	resp.Skipped = skipped
	resp.TotalSamples = total
	resp.LabeledSamples = labeled
	resp.SkipReasons = topResearchCounts(skipReasons, 0)
	resp.Quality = datasetQualityFromRows(resp.Rows, resp.Normalization)
	log.Printf("[ML] Remote dataset import source=%q format=%s rows=%d imported=%d skipped=%d labelMode=%s cleanSensitive=%t", resp.Source, resp.Format, len(resp.Rows), imported, skipped, req.LabelMode, req.CleanSensitive)
	c.JSON(http.StatusOK, resp)
}

func handleMLDatasetExportGet(c *gin.Context) {
	if ml.GlobalTrainingStore == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ML training store not initialized"})
		return
	}

	items := ml.GlobalTrainingStore.AllSamplesWithIndex()
	rows := make([]remoteDatasetRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, trainingSampleToRemoteDatasetRow(item.Index, item.Sample))
	}
	total, labeled := ml.GlobalTrainingStore.Status()
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
	if ml.GlobalTrainingStore == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ML training store not initialized"})
		return
	}

	cleared := ml.GlobalTrainingStore.Clear()
	if err := ml.GlobalTrainingStore.Flush(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cleared training store but failed to persist: " + err.Error()})
		return
	}

	total, labeled := ml.GlobalTrainingStore.Status()
	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"cleared":        cleared,
		"totalSamples":   total,
		"labeledSamples": labeled,
	})
}

func bindRemoteDatasetRequest(c *gin.Context) (remoteDatasetRequest, bool) {
	return bindRemoteDatasetRequestWithLimit(c, remoteDatasetRequestBodyLimitBytes)
}

func bindRemoteDatasetRequestWithLimit(c *gin.Context, requestBodyLimit int64) (remoteDatasetRequest, bool) {
	var req remoteDatasetRequest
	if c.Request != nil && c.Request.Body != nil && requestBodyLimit > 0 {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, requestBodyLimit)
	}
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
	return pullRemoteDatasetWithClient(req, nil)
}

func pullRemoteDatasetWithClient(req remoteDatasetRequest, client *http.Client) (*remoteDatasetResponse, error) {
	return pullRemoteDatasetWithRecordLimit(req, client, remoteDatasetAbsoluteRecordLimit)
}

func pullRemoteDatasetWithRecordLimit(req remoteDatasetRequest, client *http.Client, recordLimit int) (*remoteDatasetResponse, error) {
	if recordLimit <= 0 || recordLimit > remoteDatasetAbsoluteRecordLimit {
		recordLimit = remoteDatasetAbsoluteRecordLimit
	}
	downloaded, contentType, source, err := loadRemoteDatasetPayload(req, client)
	if err != nil {
		return nil, err
	}
	if looksLikeHTMLDataset(downloaded, contentType) {
		if source == "" {
			source = req.URL
		}
		return nil, fmt.Errorf("dataset source %q looks like an HTML landing page; please use a raw file URL or import a local file instead", source)
	}

	payloads, parseWarnings, err := expandRemoteDatasetPayloadsWithWarnings(downloaded, contentType, source, 0)
	if err != nil {
		return nil, err
	}

	storeLimit := req.Limit
	if req.ImportAll {
		storeLimit = recordLimit
	}
	records := make([]remoteDatasetRecord, 0, min(storeLimit, recordLimit))
	totalRecords := 0
	recordLimitTruncated := false
	format := ""
	for _, payload := range payloads {
		remainingRecords := recordLimit - totalRecords
		if remainingRecords <= 0 {
			recordLimitTruncated = true
			parseWarnings = append(parseWarnings, remoteDatasetParseWarning{Source: payload.Source, Reason: "record_limit_truncated"})
			break
		}
		remainingStored := storeLimit - len(records)
		if remainingStored < 0 {
			remainingStored = 0
		}
		parseResult, parseErr := parseRemoteDatasetRecordsWithLimits(payload.Data, req.Format, payload.Source, remoteDatasetParseLimits{
			MaxRecords:   remainingRecords,
			StoreRecords: remainingStored,
		})
		if parseErr != nil {
			if len(payloads) == 1 {
				return nil, parseErr
			}
			parseWarnings = append(parseWarnings, remoteDatasetParseWarning{Source: payload.Source, Reason: parseErr.Error(), Count: 1})
			continue
		}
		if parseResult.Total == 0 {
			parseWarnings = append(parseWarnings, remoteDatasetParseWarning{Source: payload.Source, Reason: "no_records", Count: 1})
			continue
		}
		totalRecords += parseResult.Total
		records = append(records, parseResult.Records...)
		format = mergeDatasetFormat(format, parseResult.Format)
		if parseResult.Truncated {
			recordLimitTruncated = true
			parseWarnings = append(parseWarnings, remoteDatasetParseWarning{Source: payload.Source, Reason: "record_limit_truncated"})
			break
		}
	}
	if totalRecords == 0 {
		return nil, errors.New("no dataset records found in payload")
	}
	if format == "" {
		format = normalizeRemoteDatasetFormat(req.Format, source)
	}

	rows := make([]remoteDatasetRow, 0, len(records))
	for _, record := range records {
		row := buildRemoteDatasetRow(record, req.LabelMode, req.CleanSensitive)
		if ml.GlobalTrainingStore != nil {
			row.Duplicate = ml.GlobalTrainingStore.HasExactCommand(row.Comm, row.Args)
		}
		rows = append(rows, row)
	}

	truncated := recordLimitTruncated || len(rows) < totalRecords
	if !req.ImportAll && len(rows) < totalRecords {
		parseWarnings = append(parseWarnings, remoteDatasetParseWarning{Source: source, Reason: "limit_truncated", Count: totalRecords - len(rows)})
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

	resp := &remoteDatasetResponse{
		Source:            source,
		Format:            format,
		ContentType:       contentType,
		Total:             totalRecords,
		TotalIsLowerBound: recordLimitTruncated,
		Limit:             limit,
		RecordLimit:       recordLimit,
		Truncated:         truncated,
		ParseWarnings:     parseWarnings,
		Rows:              rows,
	}
	applyRemoteDatasetResponseStats(resp, req.LabelMode, req.CleanSensitive)
	return resp, nil
}

func loadRemoteDatasetPayload(req remoteDatasetRequest, client *http.Client) ([]byte, string, string, error) {
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
	downloaded, contentType, err := downloadRemoteDatasetWithClient(req.URL, client)
	if err != nil {
		return nil, "", "", err
	}
	source := req.URL
	if req.SourceName != "" {
		source = req.SourceName
	}
	return downloaded, contentType, source, nil
}
