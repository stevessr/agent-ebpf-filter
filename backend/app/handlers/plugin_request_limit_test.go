package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPluginJSONRequestLimitReturns413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/plugin", func(c *gin.Context) {
		var payload map[string]any
		status, err := bindPluginJSON(c, &payload, 32)
		if err != nil {
			c.Status(status)
			return
		}
		c.Status(http.StatusNoContent)
	})
	body := append([]byte(`{"value":"`), bytes.Repeat([]byte("x"), 32)...)
	body = append(body, []byte(`"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/plugin", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}
