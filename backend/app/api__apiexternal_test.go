package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend_test.go section apiexternal_test.go ----

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

func TestExternalOpenAPISpecValidatesWithLowercaseMethods(t *testing.T) {
	spec := buildExternalOpenAPISpec()

	// The kin-openapi typed model serializes HTTP methods in lowercase, and the
	// /health path must carry a GET operation.
	health := spec.Paths.Find("/health")
	if health == nil || health.Get == nil {
		t.Fatalf("OpenAPI spec missing GET /health: %+v", health)
	}

	// /agentsight/events carries both GET and POST on the same path item.
	events := spec.Paths.Find("/agentsight/events")
	if events == nil || events.Get == nil || events.Post == nil {
		t.Fatalf("OpenAPI spec missing GET+POST /agentsight/events: %+v", events)
	}

	// The whole document must satisfy the OpenAPI 3.0.3 schema.
	if err := spec.Validate(context.Background()); err != nil {
		t.Fatalf("OpenAPI spec failed validation: %v", err)
	}
}
