package app

import (
	"bufio"
	"context"
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
	Active        bool   `json:"active"`
	Stopping      bool   `json:"stopping"`
	Path          string `json:"path,omitempty"`
	DefaultPath   string `json:"defaultPath"`
	StartedAt     string `json:"startedAt,omitempty"`
	Count         int64  `json:"count"`
	EnqueuedTotal uint64 `json:"enqueuedTotal"`
	FailedTotal   uint64 `json:"failedTotal"`
	DroppedTotal  uint64 `json:"droppedTotal"`
	Pending       uint64 `json:"pending"`
	QueueLen      int    `json:"queueLen"`
	QueueCap      int    `json:"queueCap"`
	LastFlushedAt string `json:"lastFlushedAt,omitempty"`
	LastError     string `json:"lastError,omitempty"`
}

type eventRecordingState struct {
	lifecycleMu sync.Mutex
	mu          sync.RWMutex
	queue       chan CapturedEventRecord
	stopCh      chan struct{}
	done        chan struct{}
	started     bool
	stopping    bool
	queueCap    int
	path        string
	startedAt   time.Time
	count       int64
	enqueued    uint64
	failed      uint64
	dropped     uint64
	lastFlushAt time.Time
	lastError   string
	terminalErr error
}

var eventRecordingStore = newEventRecordingState()

const (
	eventRecordingQueueSize      = 2048
	eventRecordingBufferBytes    = 256 * 1024
	eventRecordingFlushBatch     = 128
	eventRecordingFlushInterval  = 250 * time.Millisecond
	eventRecordingMaxRecordBytes = 4*1024*1024 - 1
	eventRecordingStopTimeout    = 5 * time.Second
)

var errEventRecordingRecordTooLarge = errors.New("event recording record exceeds the JSONL line size limit")

func newEventRecordingState() *eventRecordingState {
	return &eventRecordingState{}
}

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
	if s == nil {
		return eventRecordingStatus{DefaultPath: defaultEventRecordingPath()}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statusLocked()
}

func (s *eventRecordingState) Start(path string, truncate bool) (eventRecordingStatus, error) {
	return s.StartContext(context.Background(), path, truncate)
}

func (s *eventRecordingState) StartContext(ctx context.Context, path string, truncate bool) (eventRecordingStatus, error) {
	return s.startAtRootContext(ctx, runtimeRecordingsRoot(), path, truncate)
}

func (s *eventRecordingState) startAtRoot(root, path string, truncate bool) (eventRecordingStatus, error) {
	return s.startAtRootContext(context.Background(), root, path, truncate)
}

func (s *eventRecordingState) startAtRootContext(ctx context.Context, root, path string, truncate bool) (eventRecordingStatus, error) {
	if s == nil {
		return eventRecordingStatus{}, errors.New("event recording state is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	// A replacement generation never overlaps the previous writer. Ignore a
	// previous terminal write error here: the old generation is fully closed and
	// the new explicit start should be allowed to recover.
	stopStatus, stopErr := s.stopLocked(ctx)
	if errors.Is(stopErr, context.Canceled) || errors.Is(stopErr, context.DeadlineExceeded) {
		return stopStatus, stopErr
	}
	if err := ctx.Err(); err != nil {
		return stopStatus, err
	}

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
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return eventRecordingStatus{}, err
	}
	if info.Size() < 0 || info.Size() > eventReplayMaxFileBytes {
		_ = file.Close()
		return eventRecordingStatus{}, fmt.Errorf("%w: %d bytes (limit %d)", errRecordingFileTooLarge, info.Size(), eventReplayMaxFileBytes)
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return stopStatus, err
	}

	queue := make(chan CapturedEventRecord, eventRecordingQueueSize)
	stopCh := make(chan struct{})
	done := make(chan struct{})
	s.mu.Lock()
	s.queue = queue
	s.stopCh = stopCh
	s.done = done
	s.started = true
	s.stopping = false
	s.queueCap = cap(queue)
	s.path = absPath
	s.startedAt = time.Now().UTC()
	s.count = 0
	s.enqueued = 0
	s.failed = 0
	s.dropped = 0
	s.lastFlushAt = time.Time{}
	s.lastError = ""
	s.terminalErr = nil
	status := s.statusLocked()
	s.mu.Unlock()

	go s.runGeneration(file, info.Size(), queue, stopCh, done)
	return status, nil
}

func (s *eventRecordingState) Stop() (eventRecordingStatus, error) {
	return s.StopContext(context.Background())
}

func (s *eventRecordingState) StopContext(ctx context.Context) (eventRecordingStatus, error) {
	if s == nil {
		return eventRecordingStatus{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.stopLocked(ctx)
}

func (s *eventRecordingState) stopLocked(ctx context.Context) (eventRecordingStatus, error) {
	s.mu.Lock()
	if !s.started {
		status := s.statusLocked()
		err := s.terminalErr
		s.mu.Unlock()
		return status, err
	}
	done := s.done
	stopCh := s.stopCh
	requestStop := !s.stopping
	s.queue = nil
	s.stopping = true
	status := s.statusLocked()
	s.mu.Unlock()
	if requestStop && stopCh != nil {
		close(stopCh)
	}
	if done == nil {
		return status, nil
	}
	select {
	case <-done:
		status = s.Status()
		s.mu.RLock()
		err := s.terminalErr
		s.mu.RUnlock()
		return status, err
	case <-ctx.Done():
		return status, ctx.Err()
	}
}

func (s *eventRecordingState) Record(record CapturedEventRecord) {
	if s == nil || record.Event == nil {
		return
	}
	s.mu.Lock()
	if s.queue == nil {
		s.mu.Unlock()
		return
	}
	select {
	case s.queue <- record:
		s.enqueued++
	default:
		s.dropped++
		s.lastError = "event recording queue is full"
	}
	s.mu.Unlock()
}

func (s *eventRecordingState) statusLocked() eventRecordingStatus {
	status := eventRecordingStatus{
		Active:        s.queue != nil,
		Stopping:      s.stopping,
		Path:          s.path,
		DefaultPath:   defaultEventRecordingPath(),
		Count:         s.count,
		EnqueuedTotal: s.enqueued,
		FailedTotal:   s.failed,
		DroppedTotal:  s.dropped,
		QueueCap:      s.queueCap,
		LastError:     s.lastError,
	}
	if s.queue != nil {
		status.QueueLen = len(s.queue)
	}
	persisted := s.count
	if persisted < 0 {
		persisted = 0
	}
	completed := uint64(persisted) + s.failed
	if s.enqueued > completed {
		status.Pending = s.enqueued - completed
	}
	if !s.startedAt.IsZero() {
		status.StartedAt = s.startedAt.UTC().Format(time.RFC3339Nano)
	}
	if !s.lastFlushAt.IsZero() {
		status.LastFlushedAt = s.lastFlushAt.UTC().Format(time.RFC3339Nano)
	}
	return status
}

func (s *eventRecordingState) runGeneration(
	file *os.File,
	initialSize int64,
	queue <-chan CapturedEventRecord,
	stopCh <-chan struct{},
	done chan struct{},
) {
	writer := bufio.NewWriterSize(file, eventRecordingBufferBytes)
	ticker := time.NewTicker(eventRecordingFlushInterval)
	defer ticker.Stop()
	logicalSize := initialSize
	pending := 0
	var terminalErr error

	flushPending := func() error {
		if pending == 0 {
			return nil
		}
		count := pending
		pending = 0
		if err := writer.Flush(); err != nil {
			s.noteRecordingFailure(uint64(count), err)
			return err
		}
		s.noteRecordingPersisted(int64(count))
		return nil
	}

	process := func(record CapturedEventRecord) error {
		payload, err := marshalEventRecordingRecord(record)
		if err != nil {
			s.noteRecordingFailure(1, err)
			return nil
		}
		recordBytes := int64(len(payload) + 1)
		if logicalSize > eventReplayMaxFileBytes-recordBytes {
			err := fmt.Errorf("%w: recording reached %d bytes", errRecordingFileTooLarge, eventReplayMaxFileBytes)
			s.noteRecordingFailure(1, err)
			return err
		}
		if _, err := writer.Write(payload); err != nil {
			s.noteRecordingFailure(1, err)
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			s.noteRecordingFailure(1, err)
			return err
		}
		logicalSize += recordBytes
		pending++
		if pending >= eventRecordingFlushBatch || writer.Buffered() >= eventRecordingBufferBytes/2 {
			return flushPending()
		}
		return nil
	}

	drain := func(reason error) {
		for {
			select {
			case record := <-queue:
				if reason != nil {
					s.noteRecordingFailure(1, reason)
					continue
				}
				if err := process(record); err != nil {
					terminalErr = errors.Join(terminalErr, err)
					reason = err
					s.stopAcceptingRecordingGeneration(queue, err)
				}
			default:
				return
			}
		}
	}

	defer func() {
		if err := flushPending(); err != nil {
			terminalErr = errors.Join(terminalErr, err)
		}
		if err := file.Close(); err != nil {
			terminalErr = errors.Join(terminalErr, err)
			s.noteRecordingError(err)
		}
		s.finishRecordingGeneration(done, terminalErr)
	}()

	for {
		select {
		case <-stopCh:
			drain(nil)
			return
		default:
		}
		select {
		case <-stopCh:
			drain(nil)
			return
		case <-ticker.C:
			if err := flushPending(); err != nil {
				terminalErr = errors.Join(terminalErr, err)
				s.stopAcceptingRecordingGeneration(queue, err)
				drain(err)
				return
			}
		case record := <-queue:
			if err := process(record); err != nil {
				terminalErr = errors.Join(terminalErr, err)
				s.stopAcceptingRecordingGeneration(queue, err)
				drain(err)
				return
			}
		}
	}
}

func marshalEventRecordingRecord(record CapturedEventRecord) ([]byte, error) {
	if record.Event == nil {
		return nil, errors.New("event recording record has no event")
	}
	record = normalizeCapturedEventRecord(record)
	payload, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	if len(payload) > eventRecordingMaxRecordBytes {
		return nil, fmt.Errorf("%w: %d bytes (limit %d)", errEventRecordingRecordTooLarge, len(payload), eventRecordingMaxRecordBytes)
	}
	return payload, nil
}

func (s *eventRecordingState) stopAcceptingRecordingGeneration(queue <-chan CapturedEventRecord, err error) {
	s.mu.Lock()
	if s.queue == queue {
		s.queue = nil
		s.stopping = true
	}
	if err != nil {
		s.lastError = err.Error()
	}
	s.mu.Unlock()
}

func (s *eventRecordingState) noteRecordingPersisted(count int64) {
	if count <= 0 {
		return
	}
	s.mu.Lock()
	s.count += count
	s.lastFlushAt = time.Now().UTC()
	s.mu.Unlock()
}

func (s *eventRecordingState) noteRecordingFailure(count uint64, err error) {
	if count == 0 && err == nil {
		return
	}
	s.mu.Lock()
	s.failed += count
	if err != nil {
		s.lastError = err.Error()
	}
	s.mu.Unlock()
}

func (s *eventRecordingState) noteRecordingError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.lastError = err.Error()
	s.mu.Unlock()
}

func (s *eventRecordingState) finishRecordingGeneration(done chan struct{}, terminalErr error) {
	s.mu.Lock()
	if s.done == done {
		s.queue = nil
		s.stopCh = nil
		s.done = nil
		s.started = false
		s.stopping = false
		s.terminalErr = terminalErr
		if terminalErr != nil {
			s.lastError = terminalErr.Error()
		}
	}
	s.mu.Unlock()
	close(done)
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
	ctx, cancel := context.WithTimeout(context.Background(), eventRecordingStopTimeout)
	defer cancel()
	status, err := eventRecordingStore.StartContext(ctx, req.Path, req.Truncate)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, errRecordingFileTooLarge) {
			code = http.StatusRequestEntityTooLarge
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, gin.H{"error": err.Error(), "status": status})
		return
	}
	c.JSON(200, status)
}

func handleStopEventRecording(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), eventRecordingStopTimeout)
	defer cancel()
	status, err := eventRecordingStore.StopContext(ctx)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, gin.H{"error": err.Error(), "status": status})
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
