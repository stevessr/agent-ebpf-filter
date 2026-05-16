package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExternalAPIRoutesExposeHealthAndOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerExternalAPIRoutes(router.Group("/api/v1"))

	for _, tc := range []struct {
		path string
		key  string
	}{
		{path: "/api/v1/health", key: "apiVersion"},
		{path: "/api/v1/openapi.json", key: "openapi"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", tc.path, rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s returned invalid JSON: %v", tc.path, err)
		}
		if _, ok := body[tc.key]; !ok {
			t.Fatalf("%s response missing %q: %+v", tc.path, tc.key, body)
		}
	}
}

func TestExternalOpenAPISpecUsesLowercaseHTTPMethods(t *testing.T) {
	paths, ok := buildExternalOpenAPISpec()["paths"].(gin.H)
	if !ok {
		t.Fatal("OpenAPI spec missing paths")
	}
	health, ok := paths["/health"].(gin.H)
	if !ok {
		t.Fatal("OpenAPI spec missing /health")
	}
	if _, ok := health["get"]; !ok {
		t.Fatalf("expected lowercase get method, got %+v", health)
	}
	if _, ok := health["GET"]; ok {
		t.Fatalf("OpenAPI method keys must be lowercase, got %+v", health)
	}
}
