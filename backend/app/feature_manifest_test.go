package app

import (
	"agent-ebpf-filter/app/tls"
	codexhandlers "agent-ebpf-filter/codex/capture/handlers"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type recordingCodexCaptureSink struct {
	events int
}

func (sink *recordingCodexCaptureSink) HandleCaptureEvent(codexhandlers.Event) {
	sink.events++
}

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

func TestSystemRunRouteEnforcesRuntimeGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousRuntime := runtimeSettingsStore
	t.Cleanup(func() { runtimeSettingsStore = previousRuntime })

	tests := []struct {
		name        string
		enabled     bool
		wantStatus  int
		wantFeature string
	}{
		{name: "disabled", enabled: false, wantStatus: http.StatusForbidden, wantFeature: "system_run"},
		{name: "enabled reaches handler", enabled: true, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{SystemRunEnabled: tt.enabled}}
			router := gin.New()
			registerSystemRoutes(router.Group("/system"), newFeatureRegistry())

			req := httptest.NewRequest(http.MethodPost, "/system/run", strings.NewReader("{"))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", resp.Code, tt.wantStatus, resp.Body.String())
			}
			if tt.wantFeature != "" {
				var payload struct {
					Feature string `json:"feature"`
				}
				if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
					t.Fatalf("decode gate response: %v", err)
				}
				if payload.Feature != tt.wantFeature {
					t.Fatalf("feature = %q, want %q", payload.Feature, tt.wantFeature)
				}
			}
		})
	}
}

func TestSystemRunRouteRejectsCompiledOutFeature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wasCompiledIn := compiledFeatureIDs[FeatureSystemRun]
	delete(compiledFeatureIDs, FeatureSystemRun)
	t.Cleanup(func() {
		if wasCompiledIn {
			compiledFeatureIDs[FeatureSystemRun] = true
		} else {
			delete(compiledFeatureIDs, FeatureSystemRun)
		}
	})

	router := gin.New()
	registerSystemRoutes(router.Group("/system"), newFeatureRegistry())
	req := httptest.NewRequest(http.MethodPost, "/system/run", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusNotImplemented, resp.Body.String())
	}
	var payload struct {
		Feature    FeatureID `json:"feature"`
		CompiledIn bool      `json:"compiledIn"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode compiled-out response: %v", err)
	}
	if payload.Feature != FeatureSystemRun || payload.CompiledIn {
		t.Fatalf("compiled-out payload = %#v", payload)
	}
}

func TestTLSCaptureRoutesEnforceRuntimeGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousRuntime := runtimeSettingsStore
	t.Cleanup(func() { runtimeSettingsStore = previousRuntime })

	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{TlsCaptureEnabled: enabled}}
			router := gin.New()
			registerTLSCaptureFeatureRoutes(router.Group(""), newFeatureRegistry(), nil, nil, nil, nil)

			tests := []struct {
				path              string
				body              string
				enabledWantStatus int
			}{
				{path: "/tls-capture/start", body: "{}", enabledWantStatus: http.StatusInternalServerError},
				{path: "/codex/capture", body: "{", enabledWantStatus: http.StatusBadRequest},
			}
			for _, tt := range tests {
				req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
				resp := httptest.NewRecorder()
				router.ServeHTTP(resp, req)

				wantStatus := http.StatusForbidden
				if enabled {
					wantStatus = tt.enabledWantStatus
				}
				if resp.Code != wantStatus {
					t.Fatalf("POST %s status = %d, want %d; body=%s", tt.path, resp.Code, wantStatus, resp.Body.String())
				}
				if !enabled {
					var payload struct {
						Feature string `json:"feature"`
					}
					if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
						t.Fatalf("decode %s gate response: %v", tt.path, err)
					}
					if payload.Feature != "tls_capture" {
						t.Fatalf("POST %s feature = %q, want tls_capture", tt.path, payload.Feature)
					}
				}
			}
		})
	}
}

func TestTLSCaptureWebSocketRouteEnforcesRuntimeGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousRuntime := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{TlsCaptureEnabled: false}}
	t.Cleanup(func() { runtimeSettingsStore = previousRuntime })

	router := gin.New()
	registerWebSocketRoutes(router, nil, newFeatureRegistry(), nil)
	req := httptest.NewRequest(http.MethodGet, "/ws/tls-capture", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
}

func TestTLSCaptureControllerSerializesCodexIngressWithRuntimeDisable(t *testing.T) {
	enabled := true
	controller := tls.NewTLSCaptureController(tls.NewTLSCaptureStore(10), tls.NewTLSCaptureRuleStore(), tls.NewTLSCaptureBroadcaster())
	controller.SetEnabledCheck(func() bool { return enabled })
	sink := &recordingCodexCaptureSink{}
	gatedSink := codexhandlers.CaptureSinkFunc(func(event codexhandlers.Event) {
		controller.RunIfEnabled(func() { sink.HandleCaptureEvent(event) })
	})

	gatedSink.HandleCaptureEvent(codexhandlers.Event{})
	enabled = false
	if err := controller.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	gatedSink.HandleCaptureEvent(codexhandlers.Event{})
	if sink.events != 1 {
		t.Fatalf("captured events = %d, want only the pre-disable event", sink.events)
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
