package main

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

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
