package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"agent-ebpf-filter/app/types"
)

// fakePluginService is a PluginService whose behaviour is supplied per test
// through function fields; unset fields return zero values.
type fakePluginService struct {
	validateID func(id string) error
	get        func(id string) (types.PluginManifest, bool)
	upsert     func(req *PluginUpsertRequest) (types.PluginManifest, error)
	delete     func(id string) error
	loadEBPF   func(ctx context.Context, id string) (types.PluginManifest, error)
	unloadEBPF func(id string)
	compile    func(ctx context.Context, id, source string) (string, []byte, error)
}

func (f *fakePluginService) ValidateID(id string) error {
	if f.validateID == nil {
		return nil
	}
	return f.validateID(id)
}
func (f *fakePluginService) List() []types.PluginManifest { return nil }
func (f *fakePluginService) Get(id string) (types.PluginManifest, bool) {
	if f.get == nil {
		return types.PluginManifest{}, false
	}
	return f.get(id)
}
func (f *fakePluginService) Source(string) (string, bool) { return "", false }
func (f *fakePluginService) Upsert(req *PluginUpsertRequest) (types.PluginManifest, error) {
	if f.upsert == nil {
		return types.PluginManifest{}, nil
	}
	return f.upsert(req)
}
func (f *fakePluginService) Delete(id string) error {
	if f.delete == nil {
		return nil
	}
	return f.delete(id)
}
func (f *fakePluginService) SetEnabled(context.Context, string, bool) (types.PluginManifest, error) {
	return types.PluginManifest{}, nil
}
func (f *fakePluginService) LoadEBPF(ctx context.Context, id string) (types.PluginManifest, error) {
	if f.loadEBPF == nil {
		return types.PluginManifest{}, nil
	}
	return f.loadEBPF(ctx, id)
}
func (f *fakePluginService) UnloadEBPF(id string) {
	if f.unloadEBPF != nil {
		f.unloadEBPF(id)
	}
}
func (f *fakePluginService) CompileUserBPF(ctx context.Context, id, source string) (string, []byte, error) {
	if f.compile == nil {
		return "", nil, nil
	}
	return f.compile(ctx, id, source)
}
func (f *fakePluginService) BPFTemplates() []types.BPFTemplate { return nil }

func withPluginHandlerDeps(t *testing.T) *fakePluginService {
	t.Helper()
	old := Deps.Plugins
	t.Cleanup(func() { Deps.Plugins = old })
	fake := &fakePluginService{validateID: func(id string) error {
		if id == "valid-plugin" {
			return nil
		}
		return fmt.Errorf("invalid plugin id")
	}}
	Deps.Plugins = fake
	return fake
}

func TestHandlePluginUpsertPassesTypedRequestAndEnforcesPathID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := withPluginHandlerDeps(t)
	var received *PluginUpsertRequest
	fake.upsert = func(request *PluginUpsertRequest) (types.PluginManifest, error) {
		received = request
		return types.PluginManifest{ID: request.ID}, nil
	}
	router := gin.New()
	router.POST("/plugins", HandlePluginUpsert)
	router.PUT("/plugins/:id", HandlePluginUpsert)

	req := httptest.NewRequest(http.MethodPost, "/plugins", strings.NewReader(`{"id":"valid-plugin","name":"Valid"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || received == nil || received.ID != "valid-plugin" {
		t.Fatalf("POST status=%d request=%+v body=%s", rec.Code, received, rec.Body.String())
	}

	received = nil
	req = httptest.NewRequest(http.MethodPut, "/plugins/valid-plugin", strings.NewReader(`{"name":"Updated"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || received == nil || received.ID != "valid-plugin" {
		t.Fatalf("PUT status=%d request=%+v body=%s", rec.Code, received, rec.Body.String())
	}

	received = nil
	req = httptest.NewRequest(http.MethodPut, "/plugins/valid-plugin", strings.NewReader(`{"id":"different-plugin"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || received != nil {
		t.Fatalf("mismatched PUT status=%d request=%+v", rec.Code, received)
	}
}

func TestHandlePluginUpsertRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withPluginHandlerDeps(t)
	router := gin.New()
	router.POST("/plugins", HandlePluginUpsert)
	body := append([]byte(`{"id":"valid-plugin","source":"`), bytes.Repeat([]byte("x"), int(pluginUpsertMaxBodyBytes))...)
	body = append(body, []byte(`"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/plugins", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlePluginDeleteUnloadsBeforeRemoving(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := withPluginHandlerDeps(t)
	order := make([]string, 0, 2)
	fake.unloadEBPF = func(string) { order = append(order, "unload") }
	fake.delete = func(string) error {
		order = append(order, "delete")
		return nil
	}
	router := gin.New()
	router.DELETE("/plugins/:id", HandlePluginDelete)
	req := httptest.NewRequest(http.MethodDelete, "/plugins/valid-plugin", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Join(order, ",") != "unload,delete" {
		t.Fatalf("status=%d order=%v", rec.Code, order)
	}
}

func TestHandleBPFCompileUsesRequestContextAndCompatibilityResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := withPluginHandlerDeps(t)
	var contextError error
	fake.compile = func(ctx context.Context, id, source string) (string, []byte, error) {
		contextError = ctx.Err()
		if id != "valid-plugin" || source != "source" {
			return "", nil, fmt.Errorf("unexpected compile request")
		}
		return "/plugins/valid-plugin/program.o", []byte("warning"), nil
	}
	router := gin.New()
	router.POST("/plugins/bpf/compile", HandleBPFCompile)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/plugins/bpf/compile", strings.NewReader(`{"id":"valid-plugin","source":"source"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !errors.Is(contextError, context.Canceled) {
		t.Fatalf("status=%d contextError=%v body=%s", rec.Code, contextError, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["objectPath"] != "/plugins/valid-plugin/program.o" || response["sourceSha256"] == "" || response["compiledAt"] == nil || response["log"] != "warning" {
		t.Fatalf("incompatible compile response: %#v", response)
	}
	if _, legacy := response["objPath"]; legacy {
		t.Fatalf("legacy response key leaked: %#v", response)
	}
}

func TestHandleBPFLoadAndUnloadDelegateLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := withPluginHandlerDeps(t)
	loaded := false
	fake.get = func(id string) (types.PluginManifest, bool) {
		if id != "valid-plugin" {
			return types.PluginManifest{}, false
		}
		return types.PluginManifest{ID: id}, true
	}
	fake.loadEBPF = func(_ context.Context, id string) (types.PluginManifest, error) {
		loaded = true
		return types.PluginManifest{ID: id}, nil
	}
	fake.unloadEBPF = func(id string) { loaded = false }
	router := gin.New()
	router.POST("/plugins/bpf/load", HandleBPFLoad)
	router.POST("/plugins/bpf/unload", HandleBPFUnload)

	for path, wantLoaded := range map[string]bool{
		"/plugins/bpf/load":   true,
		"/plugins/bpf/unload": false,
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"id":"valid-plugin"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || loaded != wantLoaded {
			t.Fatalf("%s status=%d loaded=%v body=%s", path, rec.Code, loaded, rec.Body.String())
		}
	}
}

func TestRegisterPluginRoutesGatesMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	called := false
	group := router.Group("/plugins")
	RegisterPluginRoutes(group, func(c *gin.Context) {
		called = true
		c.AbortWithStatus(http.StatusTeapot)
	})
	req := httptest.NewRequest(http.MethodPost, "/plugins/bpf/compile", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot || !called {
		t.Fatalf("mutation bypassed policy middleware: status=%d called=%v", rec.Code, called)
	}
}
