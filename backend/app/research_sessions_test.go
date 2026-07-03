package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
)

func restoreResearchV2TestState(t *testing.T) (*researchSessionStore, *researchTaskManager) {
	t.Helper()
	oldRuntime := runtimeSettingsStore
	oldArchive := capturedEventArchive
	oldSessions := researchSessionsStore
	oldTasks := researchTaskStore
	oldUploaded := agentSightUploadedEvents
	oldLoop := loopDetectionWorkerStore

	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{ResearchProcessing: ResearchProcessingSettings{
		Enabled:               true,
		MaxEvents:             100,
		QueueSize:             128,
		TimelineBucketSeconds: 60,
		TopK:                  10,
		RecentSamples:         5,
		ArtifactRetentionDays: 14,
		MaxSessionEvents:      1000,
		ExportFormats:         "jsonl,csv,bundle",
	}}}
	capturedEventArchive = newEventArchive(50)
	agentSightUploadedEvents = newAgentSightEventStore(50)
	loopDetectionWorkerStore = newLoopDetectionWorker()

	store := newResearchSessionStore(t.TempDir())
	manager := newResearchTaskManager(store)
	manager.Start(128)
	researchSessionsStore = store
	researchTaskStore = manager

	t.Cleanup(func() {
		runtimeSettingsStore = oldRuntime
		capturedEventArchive = oldArchive
		researchSessionsStore = oldSessions
		researchTaskStore = oldTasks
		agentSightUploadedEvents = oldUploaded
		loopDetectionWorkerStore = oldLoop
	})
	return store, manager
}

func TestResearchSessionHandlersBuildResultsAndExports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreResearchV2TestState(t)

	base := time.UnixMilli(1710000000000).UTC()
	capturedEventArchive.Add(normalizeCapturedEventRecord(CapturedEventRecord{ReceivedAt: base, Event: &pb.Event{
		Pid:            101,
		Ppid:           1,
		Type:           "openat",
		EventType:      pb.EventType_OPENAT,
		Comm:           "research-agent",
		Path:           "/workspace/input.txt",
		TraceId:        "trace-r",
		Decision:       "ALLOW",
		RiskScore:      12,
		RedactionLevel: "standard",
	}}))
	capturedEventArchive.Add(normalizeCapturedEventRecord(CapturedEventRecord{ReceivedAt: base.Add(time.Second), Event: &pb.Event{
		Pid:            101,
		Type:           "network_connect",
		EventType:      pb.EventType_NETWORK_CONNECT,
		Comm:           "research-agent",
		NetEndpoint:    "203.0.113.9:443",
		TraceId:        "trace-r",
		Decision:       "ALERT",
		RiskScore:      91,
		RedactionLevel: "standard",
	}}))

	tlsStore := NewTLSCaptureStore(10)
	tlsStore.Add(TLSPlaintextEvent{Type: "http_request", Timestamp: base.Add(2 * time.Second), PID: 202, Comm: "node", Method: "POST", URL: "/v1/chat", Host: "api.example.test", Body: "secret body", BodySize: 99, RedactionState: "sanitized", TraceID: "trace-r"})
	agentSightUploadedEvents.Add(agentSightExportEvent{ID: "upload-1", Timestamp: base.Add(3 * time.Second).UnixMilli(), Source: "stdio", PID: 303, Comm: "claude", TraceID: "trace-r", Data: map[string]any{"event_type": "MCP_TOOL", "target": "tool.call", "body": "must-not-persist"}})

	router := gin.New()
	registerResearchRoutes(router.Group("/research"), tlsStore)

	createBody := `{"name":"risk study","tags":["risk","risk"],"sourceFilter":{"includeTLS":true,"includeUploaded":true},"timeRange":{"sinceTime":"2024-03-09T15:59:00Z"}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/research/sessions", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session status=%d body=%s", rec.Code, rec.Body.String())
	}
	var session ResearchSession
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if session.ID == "" || len(session.Tags) != 1 {
		t.Fatalf("created session mismatch: %+v", session)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/research/sessions/"+session.ID+"/tasks", strings.NewReader(`{"action":"scan_recent","limit":10}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("queue task status=%d body=%s", rec.Code, rec.Body.String())
	}
	var task ResearchTask
	if err := json.Unmarshal(rec.Body.Bytes(), &task); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/research/tasks/"+task.TaskID, nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("get task status=%d body=%s", rec.Code, rec.Body.String())
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &task)
		if task.Status == researchTaskSucceeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if task.Status != researchTaskSucceeded || task.Records != 4 {
		t.Fatalf("task did not succeed with 4 records: %+v", task)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/research/sessions/"+session.ID+"/events?limit=10&q=research", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", rec.Code, rec.Body.String())
	}
	var eventsResp struct {
		Events []ResearchEvent `json:"events"`
		Total  int             `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &eventsResp); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if eventsResp.Total != 2 || len(eventsResp.Events) != 2 {
		t.Fatalf("filtered events mismatch: %+v", eventsResp)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/research/sessions/"+session.ID+"/results", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("results status=%d body=%s", rec.Code, rec.Body.String())
	}
	var results ResearchResults
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode results: %v", err)
	}
	if results.Summary.Total != 4 || len(results.RiskAlerts) != 1 || results.KernelRiskFeedback.MinRiskScore == 0 {
		t.Fatalf("results mismatch: %+v", results)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/research/sessions/"+session.ID+"/export?format=csv", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("csv export status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/csv") || !strings.Contains(rec.Body.String(), "research-agent") {
		t.Fatalf("csv export mismatch type=%q body=%s", rec.Header().Get("Content-Type"), rec.Body.String())
	}
}

func TestResearchTaskQueueFullAndCancelIdempotent(t *testing.T) {
	store := newResearchSessionStore(t.TempDir())
	session, err := store.Create(researchCreateSessionRequest{Name: "queue"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	manager := newResearchTaskManager(store)
	manager.queue = make(chan *researchTaskEntry, 1)
	manager.started = true

	first, err := manager.Submit(session.ID, researchTaskRequest{Action: "scan_recent", Limit: 1}, nil)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if _, err := manager.Submit(session.ID, researchTaskRequest{Action: "scan_recent", Limit: 1}, nil); !errors.Is(err, errResearchQueueFull) {
		t.Fatalf("second submit error = %v, want queue full", err)
	}
	cancelOne := manager.Cancel(first.TaskID)
	cancelTwo := manager.Cancel(first.TaskID)
	if cancelOne.Status != researchTaskCanceled || cancelTwo.Status != researchTaskCanceled {
		t.Fatalf("cancel statuses = %+v / %+v", cancelOne, cancelTwo)
	}
}

func TestResearchEventNormalizationRedactsSensitiveAgentSightData(t *testing.T) {
	uploaded := agentSightExportEvent{Timestamp: time.UnixMilli(1710000200000).UnixMilli(), Source: "stdio", PID: 44, Comm: "agent", Data: map[string]any{"event_type": "STDIO", "body": "secret", "headers": map[string]any{"authorization": "Bearer abc"}, "target": "stdout"}}
	event, ok := researchEventFromAgentSight(uploaded)
	if !ok {
		t.Fatal("expected uploaded event to normalize")
	}
	if event.EventType != "STDIO" || event.Target != "stdout" {
		t.Fatalf("event projection mismatch: %+v", event)
	}
	if event.Features["body"] != "[redacted]" || event.Features["headers"] != "[redacted]" {
		buf, _ := json.Marshal(event.Features)
		t.Fatalf("sensitive features were not redacted: %s", buf)
	}
}

func TestResearchExportBundleContainsManifest(t *testing.T) {
	store := newResearchSessionStore(t.TempDir())
	oldRuntime := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{ResearchProcessing: ResearchProcessingSettings{MaxSessionEvents: 100, ExportFormats: "jsonl,csv,bundle"}}}
	t.Cleanup(func() { runtimeSettingsStore = oldRuntime })
	session, err := store.Create(researchCreateSessionRequest{Name: "bundle"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	events := []ResearchEvent{{ID: "e1", Timestamp: time.Now().UTC().UnixMilli(), Time: time.Now().UTC().Format(time.RFC3339Nano), Source: "file", EventType: "OPENAT", PID: 1, Comm: "agent", Target: "/tmp/a", RedactionLevel: "standard"}}
	results := buildResearchResults(session.ID, events, nil)
	if err := store.ReplaceSessionEvents(session.ID, events, results, ResearchSourceFilter{}, ResearchTimeRange{}); err != nil {
		t.Fatalf("replace events: %v", err)
	}
	ref, payload, err := store.ExportArtifact(session.ID, "bundle")
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}
	if ref.Format != "bundle" || !bytes.Contains(payload, []byte("manifest.json")) {
		t.Fatalf("bundle artifact mismatch ref=%+v len=%d", ref, len(payload))
	}
}
