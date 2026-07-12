package handlers

import (
	"bytes"
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
	oldValidate := Deps.PluginValidateID
	oldDelete := Deps.PluginDelete
	oldUnload := Deps.PluginUnloadEBPF
	t.Cleanup(func() {
		Deps.PluginUpsert = oldUpsert
		Deps.PluginValidateID = oldValidate
		Deps.PluginDelete = oldDelete
		Deps.PluginUnloadEBPF = oldUnload
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
	Deps.PluginUpsert = func(value any) error {
		request, ok := value.(*PluginUpsertRequest)
		if !ok {
			return fmt.Errorf("unexpected request type %T", value)
		}
		received = request
		return nil
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
	Deps.PluginUpsert = func(any) error { return nil }
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
