package handlers

import (
	"agent-ebpf-filter/core"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
