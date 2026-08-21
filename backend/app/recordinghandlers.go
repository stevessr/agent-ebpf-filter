package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"agent-ebpf-filter/app/recording"
)

// HTTP handlers for the event-recording endpoints. Transport layer only;
// state machine and IO live in package recording.

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

func handleEventRecordingStatus(c *gin.Context) {
	c.JSON(200, recording.Default().Status())
}

func handleStartEventRecording(c *gin.Context) {
	var req eventRecordingRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, recording.ControlRequestMaxBytes)
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		writeRecordingBindError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), recording.EventRecordingStopTimeout)
	defer cancel()
	status, err := recording.Default().StartContext(ctx, req.Path, req.Truncate)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, recording.ErrFileTooLarge) {
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
	ctx, cancel := context.WithTimeout(context.Background(), recording.EventRecordingStopTimeout)
	defer cancel()
	status, err := recording.Default().StopContext(ctx)
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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, recording.ControlRequestMaxBytes)
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
	ctx, cancel := context.WithTimeout(c.Request.Context(), recording.EventReplayProcessingTimeout)
	defer cancel()
	records, resolvedPath, err := recording.ReadCapturedEventsFileAtRootContext(ctx, recording.RuntimeRecordingsRoot(), req.Path, req.Limit)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		status := http.StatusBadRequest
		if errors.Is(err, recording.ErrFileTooLarge) || errors.Is(err, recording.ErrLineTooLarge) || errors.Is(err, recording.ErrTooManyLines) {
			status = http.StatusRequestEntityTooLarge
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	graph, err := buildExecutionGraphContext(ctx, records, executionGraphFiltersFromRequest(c))
	if err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	graph.Source = "replay_file"
	c.JSON(200, gin.H{"path": resolvedPath, "events": len(records), "graph": graph})
}

func handleSaveBrowserRecording(c *gin.Context) {
	var req browserRecordingSaveRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, recording.BrowserRecordingRequestMaxBytes)
	if err := c.ShouldBindJSON(&req); err != nil {
		status := http.StatusBadRequest
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, gin.H{"error": "invalid request"})
		return
	}
	path, snapshots, err := recording.SaveBrowserRecordingExport(req.Path, req.Export)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, recording.ErrBrowserExportTooLarge) {
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
