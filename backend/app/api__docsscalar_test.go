package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestScalarDocsRendersInlineSpec(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerDocsRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/docs returned %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("/docs returned unexpected content type %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "@scalar/api-reference") {
		t.Fatal("/docs page is missing the Scalar CDN script")
	}
	if strings.Contains(body, "__OPENAPI_SPEC__") {
		t.Fatal("/docs page still contains the unsubstituted spec placeholder")
	}
	// The inlined spec must carry the OpenAPI marker so Scalar can render it.
	if !strings.Contains(body, "\"openapi\"") {
		t.Fatal("/docs page does not inline the OpenAPI document")
	}
}

func TestDocsRoutesExposeOpenAPISpec(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerDocsRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/openapi.json returned %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"openapi\"") {
		t.Fatalf("/openapi.json missing openapi field: %s", rec.Body.String())
	}
}
