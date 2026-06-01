package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/gin-gonic/gin"
)

func TestAgentSightEventsExportReferenceJSONL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreAgentSightTestState(t)

	capturedEventArchive.Add(normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.UnixMilli(1710000001000).UTC(),
		Event: &pb.Event{
			Pid:       123,
			Ppid:      1,
			Type:      "openat",
			EventType: pb.EventType_OPENAT,
			Comm:      "python",
			Path:      "/workspace/app.py",
			TraceId:   "trace-file",
		},
	}))

	tlsStore := NewTLSCaptureStore(10)
	tlsStore.Add(TLSPlaintextEvent{
		Type:           "http_request",
		Timestamp:      time.UnixMilli(1710000002000).UTC(),
		PID:            456,
		Comm:           "node",
		Method:         "POST",
		URL:            "/v1/messages",
		Host:           "api.anthropic.com",
		BodySize:       256,
		RedactionState: "sanitized",
		TraceID:        "trace-http",
	})

	router := gin.New()
	registerAgentSightRoutes(router.Group(""), tlsStore)

	req := httptest.NewRequest(http.MethodGet, "/agentsight/events?format=jsonl&limit=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /agentsight/events returned %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/x-ndjson") {
		t.Fatalf("content type = %q, want application/x-ndjson", got)
	}

	events := decodeAgentSightJSONLLines(t, rec.Body.String())
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2: %#v", len(events), events)
	}
	if events[0].Source != "file" {
		t.Fatalf("first source = %q, want file", events[0].Source)
	}
	if events[0].Data["path"] != "/workspace/app.py" {
		t.Fatalf("file path = %#v, want /workspace/app.py", events[0].Data["path"])
	}
	if events[0].Data["event_type"] != "OPENAT" {
		t.Fatalf("file event_type = %#v, want OPENAT", events[0].Data["event_type"])
	}
	if events[1].Source != "http_parser" {
		t.Fatalf("second source = %q, want http_parser", events[1].Source)
	}
	if events[1].Data["host"] != "api.anthropic.com" {
		t.Fatalf("http host = %#v, want api.anthropic.com", events[1].Data["host"])
	}
	if events[1].Data["event_type"] != "HTTP_MESSAGE" {
		t.Fatalf("http event_type = %#v, want HTTP_MESSAGE", events[1].Data["event_type"])
	}
}

func TestAgentSightCompatibilityAndExternalRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreAgentSightTestState(t)

	capturedEventArchive.Add(normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.UnixMilli(1710000100000).UTC(),
		Event: &pb.Event{
			Pid:       777,
			Type:      "execve",
			EventType: pb.EventType_EXECVE,
			Comm:      "bash",
			Path:      "/usr/bin/bash",
		},
	}))

	router := gin.New()
	registerAgentSightCompatibilityRoutes(router.Group("/api"), nil)
	registerExternalAPIRoutes(router.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodPost, "/api/events", strings.NewReader(`{"timestamp":1710000101001,"source":"process","pid":888,"comm":"claude","data":{"event":"FILE_OPEN","filepath":"/tmp/imported.txt"}}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/events returned %d: %s", rec.Code, rec.Body.String())
	}
	var uploadResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("upload response decode failed: %v", err)
	}
	if uploadResp["imported"] != float64(1) {
		t.Fatalf("upload imported = %#v, want 1", uploadResp["imported"])
	}

	req = httptest.NewRequest(http.MethodGet, "/api/events?source=process", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/events returned %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("compat content type = %q, want text/plain", got)
	}
	compatEvents := decodeAgentSightJSONLLines(t, rec.Body.String())
	if len(compatEvents) != 2 || compatEvents[0].PID != 777 || compatEvents[1].PID != 888 {
		t.Fatalf("compat events = %#v, want PID 777 and uploaded PID 888", compatEvents)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/agentsight/events?format=array&source=process", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/agentsight/events returned %d: %s", rec.Code, rec.Body.String())
	}
	var externalEvents []agentSightExportEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &externalEvents); err != nil {
		t.Fatalf("external array decode failed: %v", err)
	}
	if len(externalEvents) != 2 || externalEvents[0].Comm != "bash" || externalEvents[1].Comm != "claude" {
		t.Fatalf("external events = %#v, want bash and uploaded claude events", externalEvents)
	}
}

func TestAgentSightStatsRunnersAndAdvancedQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreAgentSightTestState(t)

	capturedEventArchive.Add(normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.UnixMilli(1710000200000).UTC(),
		Event: &pb.Event{
			Pid:       101,
			Type:      "openat",
			EventType: pb.EventType_OPENAT,
			Comm:      "python",
			Path:      "/workspace/secret.txt",
		},
	}))
	capturedEventArchive.Add(normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.UnixMilli(1710000201000).UTC(),
		Event: &pb.Event{
			Pid:       202,
			Type:      "wrapper_intercept",
			EventType: pb.EventType_WRAPPER_INTERCEPT,
			Comm:      "claude",
			Path:      "claude --dangerously-skip-permissions",
		},
	}))
	tlsStore := NewTLSCaptureStore(10)
	tlsStore.Add(TLSPlaintextEvent{
		Type:      "sse_message",
		Timestamp: time.UnixMilli(1710000202000).UTC(),
		PID:       303,
		Comm:      "node",
		Host:      "api.openai.com",
		SSEEvent:  "content_block_delta",
	})

	router := gin.New()
	registerAgentSightRoutes(router.Group(""), tlsStore)
	registerAgentSightCompatibilityRoutes(router.Group("/api"), tlsStore)
	registerExternalAPIRoutes(router.Group("/api/v1"), tlsStore)

	req := httptest.NewRequest(http.MethodGet, "/agentsight/events/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /agentsight/events/stats returned %d: %s", rec.Code, rec.Body.String())
	}
	var stats agentSightEventsStats
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("stats decode failed: %v", err)
	}
	if stats.Total != 3 || stats.ByRunner["process"] != 1 || stats.ByRunner["agent"] != 1 || stats.ByRunner["tls"] != 1 {
		t.Fatalf("stats = %+v, want process/agent/tls counts", stats)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/runners", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/runners returned %d: %s", rec.Code, rec.Body.String())
	}
	var runnersResp struct {
		Runners []agentSightRunnerStatus `json:"runners"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &runnersResp); err != nil {
		t.Fatalf("runners decode failed: %v", err)
	}
	if len(runnersResp.Runners) == 0 {
		t.Fatal("expected runner statuses")
	}
	if !agentSightTestRunnerPresent(runnersResp.Runners, "process") || !agentSightTestRunnerPresent(runnersResp.Runners, "tls") {
		t.Fatalf("runners = %+v, want process and tls runners", runnersResp.Runners)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/agentsight/events/query", strings.NewReader(`{"runner":"tls","event_types":["SSE_MESSAGE"],"filter":"openai","limit":20}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/agentsight/events/query returned %d: %s", rec.Code, rec.Body.String())
	}
	var queryResp struct {
		Events []agentSightExportEvent `json:"events"`
		Stats  agentSightEventsStats   `json:"stats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &queryResp); err != nil {
		t.Fatalf("query decode failed: %v", err)
	}
	if len(queryResp.Events) != 1 || queryResp.Events[0].Source != "sse_processor" || queryResp.Stats.ByRunner["tls"] != 1 {
		t.Fatalf("query response = %+v, want one tls/sse event", queryResp)
	}
}

func restoreAgentSightTestState(t *testing.T) {
	t.Helper()
	oldStore := runtimeSettingsStore
	oldArchive := capturedEventArchive
	oldUploaded := agentSightUploadedEvents
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{}}
	capturedEventArchive = newEventArchive(20)
	agentSightUploadedEvents = newAgentSightEventStore(20)
	t.Cleanup(func() {
		runtimeSettingsStore = oldStore
		capturedEventArchive = oldArchive
		agentSightUploadedEvents = oldUploaded
	})
}

func agentSightTestRunnerPresent(runners []agentSightRunnerStatus, id string) bool {
	for _, runner := range runners {
		if runner.ID == id {
			return true
		}
	}
	return false
}

func decodeAgentSightJSONLLines(t *testing.T, body string) []agentSightExportEvent {
	t.Helper()
	var events []agentSightExportEvent
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event agentSightExportEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("failed to decode JSONL line %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}
