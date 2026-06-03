package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFeatureManifestReflectsCompiledFeatures(t *testing.T) {
	settings := RuntimeSettings{
		ShellSessionsEnabled:    true,
		SystemRunEnabled:        true,
		HookManagementEnabled:   true,
		PolicyManagementEnabled: true,
		OtlpEnabled:             true,
		TlsCaptureEnabled:       true,
	}
	settings.DomainForwardProxy.Enabled = true
	settings.MLConfig.Enabled = true

	manifest := buildFeatureManifest(settings)
	if len(manifest.Features) != len(featureDefinitions) {
		t.Fatalf("feature count = %d, want %d", len(manifest.Features), len(featureDefinitions))
	}

	seen := map[FeatureID]FeatureManifestEntry{}
	for _, entry := range manifest.Features {
		seen[entry.ID] = entry
		if entry.CompiledIn != isFeatureCompiledIn(entry.ID) {
			t.Fatalf("%s compiledIn = %v, want %v", entry.ID, entry.CompiledIn, isFeatureCompiledIn(entry.ID))
		}
		if entry.BuildTag == "" {
			t.Fatalf("%s missing build tag", entry.ID)
		}
	}
	for _, definition := range featureDefinitions {
		entry, ok := seen[definition.id]
		if !ok {
			t.Fatalf("missing manifest entry for %s", definition.id)
		}
		if entry.RuntimeEnabled && !entry.CompiledIn {
			t.Fatalf("%s is runtime-enabled while compiled out", entry.ID)
		}
	}
}

func TestSystemFeaturesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerSystemRoutes(router.Group("/system"), newFeatureRegistry())

	req := httptest.NewRequest(http.MethodGet, "/system/features", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var payload FeatureManifestResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(payload.Features) != len(featureDefinitions) {
		t.Fatalf("feature count = %d, want %d", len(payload.Features), len(featureDefinitions))
	}
}
