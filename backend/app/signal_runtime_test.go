package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

func TestSignalProcessingWorkerUpdatesOnlyMatchingSignalsWithTTLDecay(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{SignalProcessing: SignalProcessingSettings{
		Enabled:             true,
		QueueSize:           128,
		CronIntervalSeconds: 1,
		DefaultTTLSeconds:   300,
		MaxStates:           128,
		ProtoLogCompression: "gzip",
		Rules: []SignalRule{{
			ID:         "tmp_read",
			Name:       "tmp read",
			Enabled:    true,
			Kind:       signalKindRepeatedRead,
			TTLSeconds: 300,
			Weight:     1,
			Conditions: []SignalCondition{{Field: "path", Operator: "prefix", Value: "/tmp/"}},
		}},
	}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	worker := newSignalProcessingWorker()
	base := time.Now().UTC().Add(-time.Minute)
	unmatched := CapturedEventRecord{ReceivedAt: base, Event: &pb.Event{
		Pid:       100,
		Type:      "network_connect",
		EventType: pb.EventType_NETWORK_CONNECT,
		Comm:      "codex",
		Path:      "",
	}}
	worker.processRecord(unmatched, false)
	if status := worker.Status(); status.ActiveStates != 0 || status.UpdatedTotal != 0 {
		t.Fatalf("unmatched event should not update signal state: %+v", status)
	}

	matched := CapturedEventRecord{ReceivedAt: base, Event: &pb.Event{
		Pid:       100,
		Type:      "read",
		EventType: pb.EventType_READ,
		Comm:      "codex",
		Path:      "/tmp/context.txt",
	}}
	worker.processRecord(matched, false)
	matched.ReceivedAt = base.Add(150 * time.Second)
	worker.processRecord(matched, false)

	worker.mu.RLock()
	if len(worker.states) != 1 {
		t.Fatalf("states=%d, want one repeated-read signal", len(worker.states))
	}
	var state *signalState
	for _, candidate := range worker.states {
		state = candidate
		break
	}
	worker.mu.RUnlock()
	if state == nil || state.Count != 2 {
		t.Fatalf("state count mismatch: %+v", state)
	}
	if state.Score < 1.49 || state.Score > 1.51 {
		t.Fatalf("decayed score=%f, want about 1.5", state.Score)
	}

	worker.expireNow(base.Add(451 * time.Second))
	if status := worker.Status(); status.ActiveStates != 0 || status.ExpiredTotal == 0 {
		t.Fatalf("expired TTL state not evicted: %+v", status)
	}
}

func TestSignalSelectedProgramPersistsCompressedProtoBinary(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "codex.pb.gzlog")
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{SignalProcessing: SignalProcessingSettings{
		Enabled:             false,
		QueueSize:           128,
		CronIntervalSeconds: 1,
		DefaultTTLSeconds:   300,
		MaxStates:           128,
		ProtoLogCompression: "gzip",
		SelectedPrograms: []SelectedProgramSignalLog{{
			Program: "codex",
			Enabled: true,
			Path:    logPath,
		}},
	}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	record := CapturedEventRecord{ReceivedAt: time.Now().UTC(), Event: &pb.Event{
		Pid:       2026,
		Type:      "openat",
		EventType: pb.EventType_OPENAT,
		Comm:      "codex",
		Path:      "/tmp/selected-program.txt",
	}}
	persistSignalProgramLog(record)

	frames, err := readCompressedProtoFrames(logPath, func() proto.Message {
		return &pb.ProgramSignalLogRecord{}
	})
	if err != nil {
		t.Fatalf("read compressed proto log: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames=%d, want one", len(frames))
	}
	logRecord, ok := frames[0].(*pb.ProgramSignalLogRecord)
	if !ok {
		t.Fatalf("frame type %T, want ProgramSignalLogRecord", frames[0])
	}
	if logRecord.GetProgram() != "codex" || logRecord.GetCapturedEvent().GetEvent().GetComm() != "codex" {
		t.Fatalf("unexpected log record: %+v", logRecord)
	}
	if logRecord.GetCapturedEvent().GetEvent().GetPath() != "/tmp/selected-program.txt" {
		t.Fatalf("captured path mismatch: %+v", logRecord.GetCapturedEvent().GetEvent())
	}
}

func TestSignalRuleTestAndProgramLogStatusHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "codex.pb.gzlog")

	oldStore := runtimeSettingsStore
	oldArchive := capturedEventArchive
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{SignalProcessing: SignalProcessingSettings{
		Enabled:             true,
		QueueSize:           128,
		CronIntervalSeconds: 1,
		DefaultTTLSeconds:   300,
		MaxStates:           128,
		ProtoLogCompression: "gzip",
		SelectedPrograms: []SelectedProgramSignalLog{{
			Program: "codex",
			Enabled: true,
			Path:    logPath,
		}},
	}}}
	capturedEventArchive = newEventArchive(10)
	t.Cleanup(func() {
		runtimeSettingsStore = oldStore
		capturedEventArchive = oldArchive
	})

	record := normalizeCapturedEventRecord(CapturedEventRecord{ReceivedAt: time.Now().UTC(), Event: &pb.Event{
		Pid:       9090,
		Type:      "read",
		EventType: pb.EventType_READ,
		Comm:      "codex",
		Path:      "/tmp/dry-run.txt",
	}})
	capturedEventArchive.Add(record)
	persistSignalProgramLog(record)

	router := gin.New()
	router.POST("/system/signals/rules/test", handleSignalRuleTest)
	router.GET("/system/signals/program-logs", handleSignalProgramLogs)
	router.GET("/system/signals/program-logs/download", handleSignalProgramLogDownload)

	body := bytes.NewBufferString(`{"limit":10,"rule":{"id":"tmp-read","name":"tmp read","enabled":true,"kind":"repeated_read","ttlSeconds":60,"weight":2,"conditions":[{"field":"path","operator":"prefix","value":"/tmp/"}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/system/signals/rules/test", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rule test status=%d body=%s", rec.Code, rec.Body.String())
	}
	var testResp signalRuleTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &testResp); err != nil {
		t.Fatalf("decode rule test: %v", err)
	}
	if testResp.ScannedTotal != 1 || testResp.MatchedTotal != 1 || len(testResp.Matches) != 1 {
		t.Fatalf("unexpected rule test response: %+v", testResp)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/system/signals/program-logs", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("program log status=%d body=%s", rec.Code, rec.Body.String())
	}
	var logsResp signalProgramLogsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &logsResp); err != nil {
		t.Fatalf("decode logs status: %v", err)
	}
	if len(logsResp.Logs) != 1 || !logsResp.Logs[0].Exists || logsResp.Logs[0].FrameCount != 1 {
		t.Fatalf("unexpected program log status: %+v", logsResp)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/system/signals/program-logs/download?program=codex", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("download status=%d len=%d body=%s", rec.Code, rec.Body.Len(), rec.Body.String())
	}
}
