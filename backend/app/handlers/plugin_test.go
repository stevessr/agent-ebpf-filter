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
)

func withPluginHandlerDeps(t *testing.T) {
	t.Helper()
	oldUpsert := Deps.PluginUpsert
	oldGet := Deps.PluginGet
	oldValidate := Deps.PluginValidateID
	oldDelete := Deps.PluginDelete
	oldLoad := Deps.PluginLoadEBPF
	oldUnload := Deps.PluginUnloadEBPF
	oldCompile := Deps.CompileUserBPF
	t.Cleanup(func() {
		Deps.PluginUpsert = oldUpsert
		Deps.PluginGet = oldGet
		Deps.PluginValidateID = oldValidate
		Deps.PluginDelete = oldDelete
		Deps.PluginLoadEBPF = oldLoad
		Deps.PluginUnloadEBPF = oldUnload
		Deps.CompileUserBPF = oldCompile
	})
	Deps.PluginValidateID = func(id string) error {
		if id == "valid-plugin" {
			return nil
		}
		return fmt.Errorf("invalid plugin id")
	}
}

func TestHandlePluginUpsertPassesTypedRequestAndEnforcesPathID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withPluginHandlerDeps(t)
	var received *PluginUpsertRequest
	Deps.PluginUpsert = func(value any) (any, error) {
		request, ok := value.(*PluginUpsertRequest)
		if !ok {
			return nil, fmt.Errorf("unexpected request type %T", value)
		}
		received = request
		return map[string]any{"id": request.ID}, nil
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
	Deps.PluginUpsert = func(any) (any, error) { return nil, nil }
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
	withPluginHandlerDeps(t)
	order := make([]string, 0, 2)
	Deps.PluginUnloadEBPF = func(string) { order = append(order, "unload") }
	Deps.PluginDelete = func(string) error {
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
	withPluginHandlerDeps(t)
	var contextError error
	Deps.CompileUserBPF = func(ctx context.Context, id, source string) (string, []byte, error) {
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
	withPluginHandlerDeps(t)
	loaded := false
	Deps.PluginGet = func(id string) (any, bool) {
		if id != "valid-plugin" {
			return nil, false
		}
		return map[string]any{"id": id, "loaded": loaded}, true
	}
	Deps.PluginLoadEBPF = func(_ context.Context, id string) (any, error) {
		loaded = true
		return map[string]any{"id": id, "loaded": true}, nil
	}
	Deps.PluginUnloadEBPF = func(id string) { loaded = false }
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
