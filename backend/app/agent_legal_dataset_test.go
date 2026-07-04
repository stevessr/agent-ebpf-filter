package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAgentLegalDatasetBuildsNormalizedAllowSamples(t *testing.T) {
	oldStore := globalTrainingStore
	globalTrainingStore = newAgentLegalDatasetTestStore(t, 128)
	t.Cleanup(func() { globalTrainingStore = oldStore })

	resp, samples := buildAgentLegalDatasetResponse(32)
	if resp.Total != 32 {
		t.Fatalf("Total = %d, want 32", resp.Total)
	}
	if len(samples) != 32 || len(resp.Rows) != 32 {
		t.Fatalf("samples/rows = %d/%d, want 32/32", len(samples), len(resp.Rows))
	}
	if len(resp.Families) < 3 {
		t.Fatalf("families = %#v, want several common agent behavior families", resp.Families)
	}
	if resp.Normalization.SampleCount != 32 || resp.Normalization.FeatureDim != FeatureDim {
		t.Fatalf("normalization report = %#v", resp.Normalization)
	}
	if resp.Normalization.BelowZeroValues != 0 || resp.Normalization.AboveOneValues != 0 || resp.Normalization.NonFiniteValues != 0 {
		t.Fatalf("features are not normalized/bounded: %#v", resp.Normalization)
	}
	for i, sample := range samples {
		if sample.Label != 0 || sample.UserLabel != "agent-legal" {
			t.Fatalf("sample %d label/userLabel = %d/%q, want ALLOW/agent-legal", i, sample.Label, sample.UserLabel)
		}
		if strings.TrimSpace(sample.CommandLine) == "" || sample.Comm == "" {
			t.Fatalf("sample %d missing command identity: %#v", i, sample)
		}
		for d, v := range sample.Features {
			if v < 0 || v > 1 {
				t.Fatalf("sample %d feature %d = %f, want [0,1]", i, d, v)
			}
		}
	}
}

func TestHandleMLAgentLegalDatasetImportsSamples(t *testing.T) {
	oldStore := globalTrainingStore
	globalTrainingStore = newAgentLegalDatasetTestStore(t, 128)
	t.Cleanup(func() { globalTrainingStore = oldStore })

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/config/ml/datasets/agent-legal", strings.NewReader(`{"limit": 24, "import": true}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handleMLAgentLegalDatasetPost(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp agentLegalDatasetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Imported != 24 || resp.LabeledSamples != 24 || resp.TotalSamples != 24 {
		t.Fatalf("import summary = imported %d labeled %d total %d", resp.Imported, resp.LabeledSamples, resp.TotalSamples)
	}
	for _, sample := range globalTrainingStore.LabeledSamples() {
		if sample.Label != 0 || sample.UserLabel != "agent-legal" {
			t.Fatalf("imported sample = label %d userLabel %q, want ALLOW/agent-legal", sample.Label, sample.UserLabel)
		}
	}
}

func TestSELinuxPolicyDatasetBuildsLabeledNormalizedSamples(t *testing.T) {
	oldStore := globalTrainingStore
	globalTrainingStore = newAgentLegalDatasetTestStore(t, 128)
	t.Cleanup(func() { globalTrainingStore = oldStore })

	resp, samples := buildSELinuxPolicyDatasetResponse(64)
	if resp.Total == 0 || len(samples) != resp.Total || len(resp.Rows) != resp.Total {
		t.Fatalf("SELinux dataset shape mismatch total=%d samples=%d rows=%d", resp.Total, len(samples), len(resp.Rows))
	}
	if resp.Source != "builtin-selinux-policy-rules" {
		t.Fatalf("source = %q, want builtin-selinux-policy-rules", resp.Source)
	}
	if len(resp.Families) < 4 {
		t.Fatalf("families = %#v, want multiple SELinux rule families", resp.Families)
	}
	seenLabels := map[int32]bool{}
	for i, sample := range samples {
		seenLabels[sample.Label] = true
		if sample.UserLabel != "selinux-policy" {
			t.Fatalf("sample %d userLabel = %q, want selinux-policy", i, sample.UserLabel)
		}
		if sample.Comm != "selinux-rule" || !strings.HasPrefix(sample.CommandLine, "selinux-rule ") {
			t.Fatalf("sample %d command identity mismatch: %#v", i, sample)
		}
		if sample.Category != "SELINUX_POLICY" {
			t.Fatalf("sample %d category = %q, want SELINUX_POLICY", i, sample.Category)
		}
		for d, v := range sample.Features {
			if v < 0 || v > 1 {
				t.Fatalf("sample %d feature %d = %f, want [0,1]", i, d, v)
			}
		}
	}
	for _, label := range []int32{0, 1, 3} {
		if !seenLabels[label] {
			t.Fatalf("SELinux dataset labels = %#v, want ALLOW/BLOCK/ALERT coverage", seenLabels)
		}
	}
	if resp.Normalization.SampleCount != len(samples) || resp.Normalization.FeatureDim != FeatureDim {
		t.Fatalf("normalization report = %#v", resp.Normalization)
	}
	if resp.Normalization.BelowZeroValues != 0 || resp.Normalization.AboveOneValues != 0 || resp.Normalization.NonFiniteValues != 0 {
		t.Fatalf("SELinux features are not normalized/bounded: %#v", resp.Normalization)
	}
}

func TestHandleMLSELinuxPolicyDatasetImportsSamples(t *testing.T) {
	oldStore := globalTrainingStore
	globalTrainingStore = newAgentLegalDatasetTestStore(t, 128)
	t.Cleanup(func() { globalTrainingStore = oldStore })

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/config/ml/datasets/selinux-policy", strings.NewReader(`{"limit": 18, "import": true}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handleMLSELinuxPolicyDatasetPost(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp agentLegalDatasetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Imported != 18 || resp.LabeledSamples != 18 || resp.TotalSamples != 18 {
		t.Fatalf("import summary = imported %d labeled %d total %d", resp.Imported, resp.LabeledSamples, resp.TotalSamples)
	}
	for _, sample := range globalTrainingStore.LabeledSamples() {
		if sample.UserLabel != "selinux-policy" || sample.Label < 0 || sample.Label > 3 {
			t.Fatalf("imported SELinux sample label/userLabel mismatch: label=%d userLabel=%q", sample.Label, sample.UserLabel)
		}
	}
}

func newAgentLegalDatasetTestStore(t *testing.T, maxSamples int) *TrainingDataStore {
	t.Helper()
	store := newTrainingDataStore(maxSamples)
	tmpDir := t.TempDir()
	store.dataDir = tmpDir
	store.persistPath = filepath.Join(tmpDir, "ml_training_data.bin")
	return store
}
