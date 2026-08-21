package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agent-ebpf-filter/app/research"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
)

func TestResearchProcessingTaskAndStatusHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldSnapshot := research.SnapshotRuntimeSettingsHook
	oldRecent := research.RecentCapturedEventsHook
	oldWorker := research.ProcessingWorker
	research.SnapshotRuntimeSettingsHook = func() core.RuntimeSettings {
		return core.RuntimeSettings{ResearchProcessing: core.ResearchProcessingSettings{
			Enabled:               false,
			MaxEvents:             100,
			QueueSize:             16,
			TimelineBucketSeconds: 60,
			TopK:                  10,
			RecentSamples:         5,
		}}
	}
	archive := core.NewEventArchive(10)
	research.RecentCapturedEventsHook = func(limit int) ([]research.CapturedEventRecord, string, error) {
		return archive.Snapshot(limit), "", nil
	}
	research.ProcessingWorker = research.NewProcessingWorker()
	worker := research.ProcessingWorker
	worker.Start(context.Background(), 16)
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = worker.Shutdown(shutdownCtx)
	})
	t.Cleanup(func() {
		research.SnapshotRuntimeSettingsHook = oldSnapshot
		research.RecentCapturedEventsHook = oldRecent
		research.ProcessingWorker = oldWorker
	})

	archive.Add(core.CapturedEventRecord{
		ReceivedAt: time.Now().UTC(),
		Event: &pb.Event{
			Pid:      321,
			Type:     "openat",
			Comm:     "research-agent",
			Path:     "/tmp/research.txt",
			TraceId:  "trace-research",
			Decision: "ALLOW",
		},
	})

	router := gin.New()
	router.GET("/system/research-processing/status", research.HandleProcessingStatus)
	router.POST("/system/research-processing/task", research.HandleProcessingTask)

	req := httptest.NewRequest(http.MethodPost, "/system/research-processing/task", bytes.NewBufferString(`{"action":"scan_recent","limit":5}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST task status=%d body=%s", rec.Code, rec.Body.String())
	}

	var status research.ResearchProcessingStatus
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status = research.ProcessingWorker.Status()
		if status.Summary.Total == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status.Summary.Total != 1 || len(status.Summary.ByComm) != 1 || status.Summary.ByComm[0].Key != "research-agent" {
		t.Fatalf("research processing status did not summarize scan: %+v", status)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/system/research-processing/status", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", rec.Code, rec.Body.String())
	}
	var decoded research.ResearchProcessingStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if decoded.Summary.Total != 1 || decoded.QueueCap != 16 {
		t.Fatalf("decoded status mismatch: %+v", decoded)
	}
}
