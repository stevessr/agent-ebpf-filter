package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestClassicDatasetsListGet(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/config/ml/datasets/classic", nil)

	handleClassicDatasetsListGet(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Datasets []string `json:"datasets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Datasets) < 3 {
		t.Fatalf("datasets = %#v, want at least 3", payload.Datasets)
	}
}

func TestClassicDatasetGetAndCache(t *testing.T) {
	classicDatasetCacheMu.Lock()
	classicDatasetCache = make(map[string]*ClassicDataset)
	classicDatasetCacheMu.Unlock()

	ds1, err := loadClassicDataset("iris")
	if err != nil {
		t.Fatalf("loadClassicDataset: %v", err)
	}
	ds1.Features[0][0] = 999

	ds2, err := loadClassicDataset("iris")
	if err != nil {
		t.Fatalf("loadClassicDataset cached: %v", err)
	}
	if ds2.Features[0][0] == 999 {
		t.Fatal("cached dataset was mutated by caller")
	}
}

func TestClassicDatasetPreviewPreprocess(t *testing.T) {
	ds, err := loadClassicDataset("wine")
	if err != nil {
		t.Fatalf("loadClassicDataset: %v", err)
	}
	processed, err := preprocessClassicDataset(ds, true, false, "label")
	if err != nil {
		t.Fatalf("preprocessClassicDataset: %v", err)
	}
	if len(processed.Features) != len(ds.Features) || len(processed.Features[0]) != len(ds.Features[0]) {
		t.Fatalf("feature dimensions changed unexpectedly")
	}
	if processed.Labels[0] != "0" {
		t.Fatalf("label encoding failed: %#v", processed.Labels)
	}
	if processed.Statistics["col_0_mean"] == 0 {
		t.Fatal("statistics not computed")
	}
}

func TestClassicDatasetPreviewValidation(t *testing.T) {
	if _, err := preprocessClassicDataset(&ClassicDataset{Name: "x"}, true, true, "none"); err == nil {
		t.Fatal("expected conflict error")
	}
	if _, err := loadClassicDataset("missing"); err == nil {
		t.Fatal("expected missing dataset error")
	}
}
