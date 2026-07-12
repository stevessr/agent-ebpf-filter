package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sys/unix"
)

// ---- moved from backend/zz_merged_backend.go section recording_event.go ----

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
	return defaultEventRecordingPathAtRoot(runtimeRecordingsRoot())
}

func defaultBrowserRecordingPath() string {
	return defaultBrowserRecordingPathAtRoot(runtimeRecordingsRoot())
}

func defaultEventRecordingPathAtRoot(root string) string {
	return filepath.Join(root, "events-"+time.Now().UTC().Format("20060102-150405.000000000")+".jsonl")
}

func defaultBrowserRecordingPathAtRoot(root string) string {
	return filepath.Join(root, "browser-memory-"+time.Now().UTC().Format("20060102-150405.000000000")+".json")
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
	return s.startAtRoot(runtimeRecordingsRoot(), path, truncate)
}

func (s *eventRecordingState) startAtRoot(root, path string, truncate bool) (eventRecordingStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rootFile, absRoot, err := openRecordingsRoot(root)
	if err != nil {
		return eventRecordingStatus{}, err
	}
	defer rootFile.Close()
	defaultName := filepath.Base(defaultEventRecordingPathAtRoot(absRoot))
	name, absPath, err := resolveRecordingTarget(absRoot, path, defaultName)
	if err != nil {
		return eventRecordingStatus{}, err
	}
	var file *os.File
	if truncate {
		file, err = createTruncatedRecording(rootFile, name)
	} else {
		file, err = openRecordingForAppend(rootFile, name)
	}
	if err != nil {
		return eventRecordingStatus{}, fmt.Errorf("open recording file: %w", err)
	}

	s.closeLocked()
	s.path = absPath
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
	records, _, err := readCapturedEventsFileAtRoot(runtimeRecordingsRoot(), path, limit)
	return records, err
}

func readCapturedEventsFileAtRoot(root, path string, limit int) ([]CapturedEventRecord, string, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	rootFile, absRoot, err := openRecordingsRoot(root)
	if err != nil {
		return nil, "", err
	}
	defer rootFile.Close()
	name, absPath, err := resolveRecordingTarget(absRoot, path, "")
	if err != nil {
		return nil, "", err
	}
	file, err := openRecordingChild(rootFile, name, unix.O_RDONLY, 0)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if info.Size() > eventReplayMaxFileBytes {
		return nil, "", fmt.Errorf("%w: %d bytes (limit %d)", errRecordingFileTooLarge, info.Size(), eventReplayMaxFileBytes)
	}

	limited := &io.LimitedReader{R: file, N: eventReplayMaxFileBytes + 1}
	scanner := bufio.NewScanner(limited)
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
		return nil, "", err
	}
	if limited.N <= 0 {
		return nil, "", fmt.Errorf("%w: file grew beyond %d bytes during replay", errRecordingFileTooLarge, eventReplayMaxFileBytes)
	}
	return records, absPath, nil
}

func saveBrowserRecordingExport(path string, payload json.RawMessage) (string, int, error) {
	return saveBrowserRecordingExportAtRoot(runtimeRecordingsRoot(), path, payload)
}

func saveBrowserRecordingExportAtRoot(root, path string, payload json.RawMessage) (string, int, error) {
	if len(payload) == 0 || string(payload) == "null" {
		return "", 0, errors.New("browser recording export is empty")
	}
	if len(payload) > browserRecordingExportMaxBytes {
		return "", 0, fmt.Errorf("%w: %d bytes (limit %d)", errBrowserRecordingTooLarge, len(payload), browserRecordingExportMaxBytes)
	}
	var normalized any
	if err := json.Unmarshal(payload, &normalized); err != nil {
		return "", 0, err
	}
	pretty, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return "", 0, err
	}
	if len(pretty)+1 > browserRecordingOutputMaxBytes {
		return "", 0, fmt.Errorf("%w after formatting: %d bytes (limit %d)", errBrowserRecordingTooLarge, len(pretty)+1, browserRecordingOutputMaxBytes)
	}
	rootFile, absRoot, err := openRecordingsRoot(root)
	if err != nil {
		return "", 0, err
	}
	defer rootFile.Close()
	defaultName := filepath.Base(defaultBrowserRecordingPathAtRoot(absRoot))
	name, absPath, err := resolveRecordingTarget(absRoot, path, defaultName)
	if err != nil {
		return "", 0, err
	}
	tempFile, tempName, err := createRecordingTemp(rootFile, "browser")
	if err != nil {
		return "", 0, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = tempFile.Close()
			_ = unix.Unlinkat(int(rootFile.Fd()), tempName, 0)
		}
	}()
	if _, err := tempFile.Write(append(pretty, '\n')); err != nil {
		return "", 0, err
	}
	if err := tempFile.Sync(); err != nil {
		return "", 0, err
	}
	if err := tempFile.Close(); err != nil {
		return "", 0, err
	}
	if err := replaceRecordingDestination(rootFile, tempName, name); err != nil {
		return "", 0, err
	}
	cleanup = false
	count := 0
	if object, ok := normalized.(map[string]any); ok {
		if snapshots, ok := object["snapshots"].([]any); ok {
			count = len(snapshots)
		}
	}
	return absPath, count, nil
}

func handleEventRecordingStatus(c *gin.Context) {
	c.JSON(200, eventRecordingStore.Status())
}

func handleStartEventRecording(c *gin.Context) {
	var req eventRecordingRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, recordingControlRequestMaxBytes)
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		writeRecordingBindError(c, err)
		return
	}
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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, recordingControlRequestMaxBytes)
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		writeRecordingBindError(c, err)
		return
	}
	if req.Path == "" {
		req.Path = c.Query("path")
	}
	if req.Limit <= 0 {
		if parsed, ok := parsePositiveIntQuery(c.Query("limit"), 10000); ok {
			req.Limit = parsed
		}
	}
	records, resolvedPath, err := readCapturedEventsFileAtRoot(runtimeRecordingsRoot(), req.Path, req.Limit)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRecordingFileTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	graph := buildExecutionGraph(records, executionGraphFiltersFromRequest(c))
	graph.Source = "replay_file"
	c.JSON(200, gin.H{"path": resolvedPath, "events": len(records), "graph": graph})
}

func handleSaveBrowserRecording(c *gin.Context) {
	var req browserRecordingSaveRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, browserRecordingRequestMaxBytes)
	if err := c.ShouldBindJSON(&req); err != nil {
		status := http.StatusBadRequest
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, gin.H{"error": "invalid request"})
		return
	}
	path, snapshots, err := saveBrowserRecordingExport(req.Path, req.Export)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errBrowserRecordingTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"path": path, "snapshots": snapshots})
}

func writeRecordingBindError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		status = http.StatusRequestEntityTooLarge
	}
	c.JSON(status, gin.H{"error": "invalid request"})
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
