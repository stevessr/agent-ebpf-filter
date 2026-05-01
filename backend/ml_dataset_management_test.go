package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
