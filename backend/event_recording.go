package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type eventRecordingRequest struct {
	Path     string `json:"path"`
	Truncate bool   `json:"truncate"`
	Limit    int    `json:"limit"`
}

type eventReplayRequest struct {
	Path  string `json:"path"`
	Limit int    `json:"limit"`
}

type browserRecordingSaveRequest struct {
	Path   string          `json:"path"`
	Export json.RawMessage `json:"export"`
}

type eventRecordingStatus struct {
	Active      bool   `json:"active"`
	Path        string `json:"path,omitempty"`
	DefaultPath string `json:"defaultPath"`
	StartedAt   string `json:"startedAt,omitempty"`
	Count       int64  `json:"count"`
}

type eventRecordingState struct {
	mu        sync.Mutex
	path      string
	file      *os.File
	writer    *bufio.Writer
	startedAt time.Time
	count     int64
}

var eventRecordingStore = &eventRecordingState{}

func defaultEventRecordingPath() string {
	return filepath.Join(runtimeSettingsDir(), "recordings", "events-"+time.Now().UTC().Format("20060102-150405")+".jsonl")
}

func defaultBrowserRecordingPath() string {
	return filepath.Join(runtimeSettingsDir(), "recordings", "browser-memory-"+time.Now().UTC().Format("20060102-150405")+".json")
}

func expandEventRecordingPath(raw string) string {
	path := strings.TrimSpace(raw)
	if path == "" {
		return defaultEventRecordingPath()
	}
	if path == "~" {
		return getRealHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(getRealHomeDir(), strings.TrimPrefix(path, "~/"))
	}
	return path
}

func (s *eventRecordingState) Status() eventRecordingStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := eventRecordingStatus{
		Active:      s.writer != nil,
		Path:        s.path,
		DefaultPath: defaultEventRecordingPath(),
		Count:       s.count,
	}
	if !s.startedAt.IsZero() {
		status.StartedAt = s.startedAt.UTC().Format(time.RFC3339Nano)
	}
	return status
}

func (s *eventRecordingState) Start(path string, truncate bool) (eventRecordingStatus, error) {
	path = expandEventRecordingPath(path)
	if strings.TrimSpace(path) == "" {
		return eventRecordingStatus{}, errors.New("recording path is empty")
	}
	if err := mkdirAllAsRealUser(filepath.Dir(path), 0o755); err != nil {
		return eventRecordingStatus{}, err
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if truncate {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return eventRecordingStatus{}, err
	}
	// Fix ownership if running as root
	if os.Getuid() == 0 {
		if uid, gid, ok := originalInvokerIDs(); ok {
			_ = os.Chown(path, int(uid), int(gid))
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
	s.path = path
	s.file = file
	s.writer = bufio.NewWriter(file)
	s.startedAt = time.Now().UTC()
	s.count = 0
	return s.statusLocked(), nil
}

func (s *eventRecordingState) Stop() (eventRecordingStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.statusLocked()
	err := s.closeLocked()
	status.Active = false
	return status, err
}

func (s *eventRecordingState) Record(record CapturedEventRecord) {
	if record.Event == nil {
		return
	}
	record = normalizeCapturedEventRecord(record)
	payload, err := json.Marshal(record)
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer == nil {
		return
	}
	if _, err := s.writer.Write(payload); err != nil {
		return
	}
	if err := s.writer.WriteByte('\n'); err != nil {
		return
	}
	if err := s.writer.Flush(); err != nil {
		return
	}
	s.count++
}

func (s *eventRecordingState) statusLocked() eventRecordingStatus {
	status := eventRecordingStatus{
		Active:      s.writer != nil,
		Path:        s.path,
		DefaultPath: defaultEventRecordingPath(),
		Count:       s.count,
	}
	if !s.startedAt.IsZero() {
		status.StartedAt = s.startedAt.UTC().Format(time.RFC3339Nano)
	}
	return status
}

func (s *eventRecordingState) closeLocked() error {
	var err error
	if s.writer != nil {
		if flushErr := s.writer.Flush(); flushErr != nil {
			err = flushErr
		}
	}
	if s.file != nil {
		if closeErr := s.file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	s.file = nil
	s.writer = nil
	return err
}

func readCapturedEventsFile(path string, limit int) ([]CapturedEventRecord, error) {
	path = expandEventRecordingPath(path)
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("replay path is empty")
	}
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	records := make([]CapturedEventRecord, 0, min(limit, 1024))
	for scanner.Scan() {
		var record CapturedEventRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil || record.Event == nil {
			continue
		}
		record = normalizeCapturedEventRecord(record)
		records = append(records, record)
		if len(records) > limit {
			copy(records, records[len(records)-limit:])
			records = records[:limit]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func saveBrowserRecordingExport(path string, payload json.RawMessage) (string, int, error) {
	path = expandEventRecordingPath(path)
	if strings.TrimSpace(path) == "" {
		path = defaultBrowserRecordingPath()
	}
	if len(payload) == 0 || string(payload) == "null" {
		return "", 0, errors.New("browser recording export is empty")
	}
	var normalized any
	if err := json.Unmarshal(payload, &normalized); err != nil {
		return "", 0, err
	}
	pretty, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return "", 0, err
	}
	if err := mkdirAllAsRealUser(filepath.Dir(path), 0o755); err != nil {
		return "", 0, err
	}
	if err := writeFileAsRealUser(path, append(pretty, '\n'), 0o600); err != nil {
		return "", 0, err
	}
	count := 0
	if object, ok := normalized.(map[string]any); ok {
		if snapshots, ok := object["snapshots"].([]any); ok {
			count = len(snapshots)
		}
	}
	return path, count, nil
}

func handleEventRecordingStatus(c *gin.Context) {
	c.JSON(200, eventRecordingStore.Status())
}

func handleStartEventRecording(c *gin.Context) {
	var req eventRecordingRequest
	_ = c.ShouldBindJSON(&req)
	status, err := eventRecordingStore.Start(req.Path, req.Truncate)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, status)
}

func handleStopEventRecording(c *gin.Context) {
	status, err := eventRecordingStore.Stop()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error(), "status": status})
		return
	}
	c.JSON(200, status)
}

func handleReplayEventRecording(c *gin.Context) {
	var req eventReplayRequest
	_ = c.ShouldBindJSON(&req)
	if req.Path == "" {
		req.Path = c.Query("path")
	}
	if req.Limit <= 0 {
		if parsed, ok := parsePositiveIntQuery(c.Query("limit"), 10000); ok {
			req.Limit = parsed
		}
	}
	records, err := readCapturedEventsFile(req.Path, req.Limit)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	graph := buildExecutionGraph(records, executionGraphFiltersFromRequest(c))
	graph.Source = "replay_file"
	c.JSON(200, gin.H{"path": expandEventRecordingPath(req.Path), "events": len(records), "graph": graph})
}

func handleSaveBrowserRecording(c *gin.Context) {
	var req browserRecordingSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	path, snapshots, err := saveBrowserRecordingExport(req.Path, req.Export)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"path": path, "snapshots": snapshots})
}

func parsePositiveIntQuery(raw string, fallback int) (int, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback, false
	}
	var parsed int
	if err := json.Unmarshal([]byte(value), &parsed); err != nil || parsed <= 0 {
		return fallback, false
	}
	return parsed, true
}
