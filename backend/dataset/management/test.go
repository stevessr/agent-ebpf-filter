package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestTrainingDataStoreClearResetsSamples(t *testing.T) {
	store := newTrainingDataStore(8)
	store.Add(TrainingSample{
		Comm:      "echo",
		Args:      []string{"hello"},
		Timestamp: time.Unix(1700000000, 0).UTC(),
	})
	store.Add(TrainingSample{
		Comm:      "rm",
		Args:      []string{"-rf", "/tmp/demo"},
		Timestamp: time.Unix(1700000001, 0).UTC(),
	})

	total, labeled := store.Status()
	if total != 2 || labeled != 0 {
		t.Fatalf("status before clear = %d/%d, want 2/0", total, labeled)
	}

	cleared := store.Clear()
	if cleared != 2 {
		t.Fatalf("Clear() = %d, want 2", cleared)
	}

	total, labeled = store.Status()
	if total != 0 || labeled != 0 {
		t.Fatalf("status after clear = %d/%d, want 0/0", total, labeled)
	}

	if samples := store.AllSamples(); len(samples) != 0 {
		t.Fatalf("AllSamples() after clear = %d, want 0", len(samples))
	}
}

func TestPullRemoteDatasetFromContentSupportsImportAll(t *testing.T) {
	raw := []byte(`{
		"rows": [
			{"commandLine": "rm -rf /tmp/demo", "label": "BLOCK"},
			{"commandLine": "echo hello", "label": "ALLOW"}
		]
	}`)

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		Content:    string(raw),
		SourceName: "export.json",
		Format:     "auto",
		Limit:      1,
		LabelMode:  "preserve",
		ImportAll:  true,
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Source != "export.json" {
		t.Fatalf("Source = %q, want export.json", resp.Source)
	}
	if resp.Format != "json" {
		t.Fatalf("Format = %q, want json", resp.Format)
	}
	if resp.ContentType != "application/json" {
		t.Fatalf("ContentType = %q, want application/json", resp.ContentType)
	}
	if resp.Total != 2 {
		t.Fatalf("Total = %d, want 2", resp.Total)
	}
	if len(resp.Rows) != 2 {
		t.Fatalf("Rows length = %d, want 2", len(resp.Rows))
	}
	if resp.Truncated {
		t.Fatalf("Truncated = true, want false for ImportAll")
	}
	if resp.Rows[0].Label != "BLOCK" || resp.Rows[1].Label != "ALLOW" {
		t.Fatalf("rows labels = %#v %#v", resp.Rows[0].Label, resp.Rows[1].Label)
	}
}

func TestHandleMLDatasetExportAndClear(t *testing.T) {
	oldStore := globalTrainingStore
	globalTrainingStore = newTrainingDataStore(8)
	tmpDir := t.TempDir()
	globalTrainingStore.dataDir = tmpDir
	globalTrainingStore.persistPath = filepath.Join(tmpDir, "ml_training_data.bin")
	t.Cleanup(func() {
		globalTrainingStore = oldStore
	})

	globalTrainingStore.Add(TrainingSample{
		Label:        1,
		Comm:         "rm",
		Args:         []string{"-rf", "/tmp/demo"},
		Category:     "FILE_DELETE",
		AnomalyScore: 0.82,
		Timestamp:    time.Unix(1700000000, 0).UTC(),
		UserLabel:    "manual",
	})

	exportRec := httptest.NewRecorder()
	exportCtx, _ := gin.CreateTestContext(exportRec)
	exportCtx.Request = httptest.NewRequest(http.MethodGet, "/config/ml/datasets/export", nil)
	handleMLDatasetExportGet(exportCtx)

	if exportRec.Code != http.StatusOK {
		t.Fatalf("export status = %d, want %d", exportRec.Code, http.StatusOK)
	}
	var exportResp remoteDatasetResponse
	if err := json.Unmarshal(exportRec.Body.Bytes(), &exportResp); err != nil {
		t.Fatalf("unmarshal export response: %v", err)
	}
	if exportResp.Source != "local-training-store" {
		t.Fatalf("export source = %q, want local-training-store", exportResp.Source)
	}
	if exportResp.Total != 1 || len(exportResp.Rows) != 1 {
		t.Fatalf("export rows = %d/%d, want 1/1", len(exportResp.Rows), exportResp.Total)
	}
	if exportResp.Rows[0].CommandLine != "rm -rf /tmp/demo" || exportResp.Rows[0].Label != "BLOCK" {
		t.Fatalf("export row = %#v", exportResp.Rows[0])
	}

	clearRec := httptest.NewRecorder()
	clearCtx, _ := gin.CreateTestContext(clearRec)
	clearCtx.Request = httptest.NewRequest(http.MethodDelete, "/config/ml/datasets", nil)
	handleMLDatasetClearDelete(clearCtx)

	if clearRec.Code != http.StatusOK {
		t.Fatalf("clear status = %d, body = %s", clearRec.Code, clearRec.Body.String())
	}
	if total, labeled := globalTrainingStore.Status(); total != 0 || labeled != 0 {
		t.Fatalf("store status after clear = %d/%d, want 0/0", total, labeled)
	}
}

func TestHandleMLSamplesPostPreservesCommandLine(t *testing.T) {
	oldStore := globalTrainingStore
	globalTrainingStore = newTrainingDataStore(8)
	t.Cleanup(func() {
		globalTrainingStore = oldStore
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/config/ml/samples", strings.NewReader(`{
		"commandLine": "bash -c \"rm -rf /tmp/demo\"",
		"label": "BLOCK"
	}`))
	handleMLSamplesPost(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	items := globalTrainingStore.AllSamplesWithIndex()
	if len(items) != 1 {
		t.Fatalf("sample count = %d, want 1", len(items))
	}
	if got := items[0].Sample.CommandLine; got != `bash -c "rm -rf /tmp/demo"` {
		t.Fatalf("stored commandLine = %q", got)
	}

	exportRec := httptest.NewRecorder()
	exportCtx, _ := gin.CreateTestContext(exportRec)
	exportCtx.Request = httptest.NewRequest(http.MethodGet, "/config/ml/samples", nil)
	handleMLSamplesGet(exportCtx)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("export status = %d, body = %s", exportRec.Code, exportRec.Body.String())
	}
	var payload struct {
		Samples []struct {
			CommandLine string `json:"commandLine"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(exportRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(payload.Samples) != 1 || payload.Samples[0].CommandLine != `bash -c "rm -rf /tmp/demo"` {
		t.Fatalf("response samples = %#v", payload.Samples)
	}
}

func TestTrainingDataStorePersistenceRestoresArgs(t *testing.T) {
	store := newTrainingDataStore(8)
	tmpDir := t.TempDir()
	store.dataDir = tmpDir
	store.persistPath = filepath.Join(tmpDir, "ml_training_data.bin")
	store.Add(TrainingSample{
		Label:        1,
		CommandLine:  `bash -c "rm -rf /tmp/demo"`,
		Comm:         "rm",
		Args:         []string{"-rf", "/tmp/demo"},
		Category:     "FILE_DELETE",
		AnomalyScore: 0.82,
		Timestamp:    time.Unix(1700000000, 0).UTC(),
		UserLabel:    "manual",
	})
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	loaded := newTrainingDataStore(8)
	loaded.dataDir = tmpDir
	loaded.persistPath = filepath.Join(tmpDir, "ml_training_data.bin")
	if err := loaded.loadFromDisk(); err != nil {
		t.Fatalf("loadFromDisk() error = %v", err)
	}

	items := loaded.AllSamplesWithIndex()
	if len(items) != 1 {
		t.Fatalf("loaded sample count = %d, want 1", len(items))
	}
	gotArgs := items[0].Sample.Args
	wantArgs := []string{"-rf", "/tmp/demo"}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("loaded args = %#v, want %#v", gotArgs, wantArgs)
	}
	for i := range wantArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Fatalf("loaded args[%d] = %q, want %q", i, gotArgs[i], wantArgs[i])
		}
	}
	if got := items[0].Sample.CommandLine; got != `bash -c "rm -rf /tmp/demo"` {
		t.Fatalf("loaded commandLine = %q, want raw commandLine", got)
	}
}

func TestBuildLLMProductionDatasetCleansTrainingSamples(t *testing.T) {
	oldStore := globalTrainingStore
	oldConfig := mlConfig
	globalTrainingStore = newTrainingDataStore(8)
	mlConfig.LlmSystemPrompt = "SYSTEM PROMPT"
	t.Cleanup(func() {
		globalTrainingStore = oldStore
		mlConfig = oldConfig
	})

	globalTrainingStore.Add(TrainingSample{
		Label:        1,
		Comm:         "rm",
		Args:         []string{"-rf", "/tmp/demo"},
		Category:     "FILE_DELETE",
		AnomalyScore: 0.82,
		Timestamp:    time.Unix(1700000000, 0).UTC(),
		UserLabel:    "manual",
	})
	globalTrainingStore.Add(TrainingSample{
		Label:        1,
		Comm:         "rm",
		Args:         []string{"-rf", "/tmp/demo"},
		Category:     "FILE_DELETE",
		AnomalyScore: 0.82,
		Timestamp:    time.Unix(1700000001, 0).UTC(),
		UserLabel:    "manual",
	})
	globalTrainingStore.Add(TrainingSample{
		Label:        3,
		Comm:         "git",
		Args:         []string{"status"},
		Category:     "SAFE",
		AnomalyScore: 0.12,
		Timestamp:    time.Unix(1700000002, 0).UTC(),
		UserLabel:    "remote-heuristic",
	})
	globalTrainingStore.Add(TrainingSample{
		Label:        -1,
		Comm:         "echo",
		Args:         []string{"hello"},
		Category:     "SAFE",
		AnomalyScore: 0.01,
		Timestamp:    time.Unix(1700000003, 0).UTC(),
		UserLabel:    "remote-import",
	})

	resp, err := buildLLMProductionDataset(llmProductionDatasetRequest{
		Limit:          10,
		AllowHeuristic: false,
		Deduplicate:    true,
	})
	if err != nil {
		t.Fatalf("buildLLMProductionDataset() error = %v", err)
	}
	if resp.Source != "local-training-store" {
		t.Fatalf("Source = %q, want local-training-store", resp.Source)
	}
	if resp.Included != 1 {
		t.Fatalf("Included = %d, want 1", resp.Included)
	}
	if resp.SkippedDuplicates != 1 {
		t.Fatalf("SkippedDuplicates = %d, want 1", resp.SkippedDuplicates)
	}
	if resp.SkippedHeuristic != 1 {
		t.Fatalf("SkippedHeuristic = %d, want 1", resp.SkippedHeuristic)
	}
	if resp.SkippedUnlabeled != 1 {
		t.Fatalf("SkippedUnlabeled = %d, want 1", resp.SkippedUnlabeled)
	}
	if len(resp.Rows) != 1 {
		t.Fatalf("Rows length = %d, want 1", len(resp.Rows))
	}

	row := resp.Rows[0]
	if row.Label != "BLOCK" {
		t.Fatalf("row.Label = %q, want BLOCK", row.Label)
	}
	if row.Messages[0].Content != "SYSTEM PROMPT" {
		t.Fatalf("system prompt = %q, want SYSTEM PROMPT", row.Messages[0].Content)
	}
	if row.Messages[1].Role != "user" || row.Messages[2].Role != "assistant" {
		t.Fatalf("unexpected message roles = %#v", row.Messages)
	}
	if row.TargetRiskScore != 95 {
		t.Fatalf("TargetRiskScore = %v, want 95", row.TargetRiskScore)
	}
	if row.TargetConfidence < 0.98 {
		t.Fatalf("TargetConfidence = %v, want >= 0.98", row.TargetConfidence)
	}
	if row.Completion == "" || row.Prompt == "" {
		t.Fatalf("prompt/completion should not be empty: %#v", row)
	}
}
