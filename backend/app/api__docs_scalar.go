package app

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Scalar-powered API reference UI.
//
// The OpenAPI document is produced by buildExternalOpenAPISpec() and inlined
// directly into the page as the Scalar `content` option. Inlining (instead of
// pointing Scalar at /api/v1/openapi.json) means the reference renders even when
// release-mode auth is enforced — the browser never makes a second, unauthenticated
// fetch that would be rejected with 401.

const scalarDocsTemplate = `<!doctype html>
<html lang="en">
  <head>
    <title>Agent eBPF Filter API Reference</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style>
      body { margin: 0; }
    </style>
  </head>
  <body>
    <div id="app"></div>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
    <script>
      Scalar.createApiReference('#app', {
        content: __OPENAPI_SPEC__,
        theme: 'default',
        layout: 'modern',
        hideDownloadButton: false,
        metaData: {
          title: 'Agent eBPF Filter API Reference',
        },
      })
    </script>
  </body>
</html>
`

// handleScalarDocs renders the Scalar API reference with the OpenAPI spec inlined.
func handleScalarDocs(c *gin.Context) {
	spec, err := json.Marshal(buildExternalOpenAPISpec())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build OpenAPI spec"})
		return
	}
	// encoding/json escapes <, >, and & to < etc. by default, so the JSON
	// is safe to embed verbatim inside a <script> element.
	page := strings.Replace(scalarDocsTemplate, "__OPENAPI_SPEC__", string(spec), 1)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

// registerDocsRoutes exposes the human-facing API reference. The docs page and
// the spec it serves describe endpoint shapes only (no secrets), so they are
// served unauthenticated for usability — matching the convention for API portals.
func registerDocsRoutes(r gin.IRouter) {
	r.GET("/docs", handleScalarDocs)
	r.GET("/openapi.json", handleExternalAPIOpenAPI)
}
