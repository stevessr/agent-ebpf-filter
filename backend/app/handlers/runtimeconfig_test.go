package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"agent-ebpf-filter/core"
)

type runtimeSettingsTestStore struct {
	mu       sync.Mutex
	settings RuntimeSettings
}

func (store *runtimeSettingsTestStore) Snapshot() RuntimeSettings {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.settings
}

func (store *runtimeSettingsTestStore) Replace(settings RuntimeSettings) {
	store.mu.Lock()
	store.settings = settings
	store.mu.Unlock()
}

func (*runtimeSettingsTestStore) RecentEvents(int) ([]CapturedEventRecord, string, error) {
	return nil, "memory", nil
}

func (s *runtimeSettingsTestStore) RecentEventsContext(_ context.Context, limit int) ([]CapturedEventRecord, string, error) {
	return s.RecentEvents(limit)
}

func (*runtimeSettingsTestStore) EventByIDContext(context.Context, string) (CapturedEventRecord, error) {
	return CapturedEventRecord{}, os.ErrNotExist
}
func (*runtimeSettingsTestStore) TruncateEventLog() error { return nil }

func TestHandleConfigRuntimePutAppliesTLSCaptureRuntimeChange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDeps := Deps
	t.Cleanup(func() { Deps = previousDeps })

	store := &runtimeSettingsTestStore{settings: RuntimeSettings{TlsCaptureEnabled: true}}
	var applied []bool
	Deps.RuntimeSettings = store
	Deps.RuntimeSettingsReplace = func(settings RuntimeSettings) (RuntimeSettings, error) {
		store.Replace(settings)
		return settings, nil
	}
	Deps.ApplyRetentionConfig = func(RuntimeSettings) {}
	Deps.ApplyRuntimeDomainForwardProxy = func(RuntimeSettings) {}
	Deps.ApplyRuntimeTLSCapture = func(settings RuntimeSettings) {
		applied = append(applied, settings.TlsCaptureEnabled)
	}
	Deps.BuildRuntimeConfigResponseFromSettings = func(settings RuntimeSettings) core.RuntimeConfigResponse {
		return core.RuntimeConfigResponse{Runtime: settings}
	}

	router := gin.New()
	router.PUT("/config/runtime", HandleConfigRuntimePut)
	for _, enabled := range []bool{false, true} {
		body := `{"tlsCaptureEnabled":false}`
		if enabled {
			body = `{"tlsCaptureEnabled":true}`
		}
		req := httptest.NewRequest(http.MethodPut, "/config/runtime", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("enabled=%v status = %d; body=%s", enabled, resp.Code, resp.Body.String())
		}
	}

	if len(applied) != 2 || applied[0] || !applied[1] {
		t.Fatalf("applied TLS capture states = %#v, want [false true]", applied)
	}
}

func TestHandleConfigRuntimePutSerializesPersistenceAndSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDeps := Deps
	t.Cleanup(func() { Deps = previousDeps })

	store := &runtimeSettingsTestStore{settings: RuntimeSettings{TlsCaptureEnabled: true}}
	firstApplyStarted := make(chan struct{})
	releaseFirstApply := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseFirstApply:
		default:
			close(releaseFirstApply)
		}
	})
	var appliedMu sync.Mutex
	applied := make([]bool, 0, 2)

	Deps.RuntimeSettings = store
	Deps.RuntimeSettingsReplace = func(settings RuntimeSettings) (RuntimeSettings, error) {
		store.Replace(settings)
		return settings, nil
	}
	Deps.ApplyRetentionConfig = func(RuntimeSettings) {}
	Deps.ApplyRuntimeDomainForwardProxy = func(RuntimeSettings) {}
	Deps.ApplyRuntimeTLSCapture = func(settings RuntimeSettings) {
		if !settings.TlsCaptureEnabled {
			close(firstApplyStarted)
			<-releaseFirstApply
		}
		appliedMu.Lock()
		applied = append(applied, settings.TlsCaptureEnabled)
		appliedMu.Unlock()
	}
	Deps.BuildRuntimeConfigResponseFromSettings = func(settings RuntimeSettings) core.RuntimeConfigResponse {
		return core.RuntimeConfigResponse{Runtime: settings}
	}

	router := gin.New()
	router.PUT("/config/runtime", HandleConfigRuntimePut)
	request := func(body string) <-chan *httptest.ResponseRecorder {
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			req := httptest.NewRequest(http.MethodPut, "/config/runtime", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			done <- resp
		}()
		return done
	}

	firstDone := request(`{"tlsCaptureEnabled":false}`)
	select {
	case <-firstApplyStarted:
	case <-time.After(time.Second):
		t.Fatal("first runtime side effect did not start")
	}
	secondDone := request(`{"tlsCaptureEnabled":true}`)
	select {
	case resp := <-secondDone:
		t.Fatalf("second update bypassed the in-flight mutation: status=%d body=%s", resp.Code, resp.Body.String())
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseFirstApply)

	for index, done := range []<-chan *httptest.ResponseRecorder{firstDone, secondDone} {
		select {
		case resp := <-done:
			if resp.Code != http.StatusOK {
				t.Fatalf("request %d status = %d; body=%s", index, resp.Code, resp.Body.String())
			}
		case <-time.After(time.Second):
			t.Fatalf("request %d did not finish", index)
		}
	}

	appliedMu.Lock()
	gotApplied := append([]bool(nil), applied...)
	appliedMu.Unlock()
	if len(gotApplied) != 2 || gotApplied[0] || !gotApplied[1] {
		t.Fatalf("applied TLS states = %#v, want [false true]", gotApplied)
	}
	if final := store.Snapshot(); !final.TlsCaptureEnabled {
		t.Fatalf("final TLS setting = false, want the second update to win")
	}
}

func TestHandleConfigRuntimePutAppliesActionRiskThresholds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDeps := Deps
	t.Cleanup(func() { Deps = previousDeps })

	store := &runtimeSettingsTestStore{settings: RuntimeSettings{MLConfig: core.MLConfig{
		AllowRiskScoreThreshold:  0.3,
		BlockConfidenceThreshold: 0.85,
	}}}
	Deps.RuntimeSettings = store
	Deps.RuntimeSettingsReplace = func(settings RuntimeSettings) (RuntimeSettings, error) {
		store.Replace(settings)
		return settings, nil
	}
	Deps.ApplyRetentionConfig = func(RuntimeSettings) {}
	Deps.ApplyRuntimeDomainForwardProxy = func(RuntimeSettings) {}
	Deps.ApplyRuntimeTLSCapture = func(RuntimeSettings) {}
	Deps.BuildRuntimeConfigResponseFromSettings = func(settings RuntimeSettings) core.RuntimeConfigResponse {
		return core.RuntimeConfigResponse{Runtime: settings}
	}

	router := gin.New()
	router.PUT("/config/runtime", HandleConfigRuntimePut)
	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/config/runtime", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp
	}

	resp := put(`{"allowRiskScoreThreshold":0.15,"blockConfidenceThreshold":0.9}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("valid thresholds status = %d; body=%s", resp.Code, resp.Body.String())
	}
	applied := store.Snapshot().MLConfig
	if applied.AllowRiskScoreThreshold != 0.15 || applied.BlockConfidenceThreshold != 0.9 {
		t.Fatalf("applied thresholds = %+v, want allow 0.15 / block 0.9", applied)
	}

	resp = put(`{"allowRiskScoreThreshold":0}`)
	if resp.Code != http.StatusOK || store.Snapshot().MLConfig.AllowRiskScoreThreshold != 0 {
		t.Fatalf("zero allow threshold status=%d settings=%+v body=%s", resp.Code, store.Snapshot().MLConfig, resp.Body.String())
	}
	applied = store.Snapshot().MLConfig

	for _, body := range []string{
		`{"allowRiskScoreThreshold":0.6}`,
		`{"blockConfidenceThreshold":0.4}`,
	} {
		resp = put(body)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("body %s status = %d, want 400; body=%s", body, resp.Code, resp.Body.String())
		}
	}
	if got := store.Snapshot().MLConfig; got != applied {
		t.Fatalf("rejected update mutated settings: %+v", got)
	}
}

func TestHandleConfigRuntimePutRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/config/runtime", HandleConfigRuntimePut)
	body := append([]byte(`{"llmSystemPrompt":"`), bytes.Repeat([]byte("x"), int(runtimeSettingsMaxBodyBytes))...)
	body = append(body, []byte(`"}`)...)
	req := httptest.NewRequest(http.MethodPut, "/config/runtime", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}
