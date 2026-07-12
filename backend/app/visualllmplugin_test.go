package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend_test.go section visualllmplugin_test.go ----

func TestParseVisualBlocksLLMContentSocketCounter(t *testing.T) {
	content := `{"trigger":"socket_connect","action":"KILL","conditions":{"id":"root","type":"AND","children":[{"id":"cond-comm","type":"CONDITION","field":"comm","operator":"==","value":"python"},{"id":"cond-port","type":"CONDITION","field":"port","operator":"==","value":4444}]},"mapMode":"COUNTER","mapKey":"pid","mapLimit":3,"reasoning":"外连端口强杀"}`

	got, err := parseVisualBlocksLLMContent(content)
	if err != nil {
		t.Fatalf("parseVisualBlocksLLMContent() error = %v", err)
	}
	if got.Trigger != "socket_connect" || got.Action != "KILL" {
		t.Fatalf("trigger/action = %s/%s, want socket_connect/KILL", got.Trigger, got.Action)
	}
	if got.MapMode != "COUNTER" || got.MapKey != "pid" || got.MapLimit != 3 {
		t.Fatalf("map = %s/%s/%d, want COUNTER/pid/3", got.MapMode, got.MapKey, got.MapLimit)
	}
	if got.Conditions.ID != "root" || len(got.Conditions.Children) != 2 {
		t.Fatalf("conditions = %#v", got.Conditions)
	}
	if got.Conditions.Children[1].Field != "port" || got.Conditions.Children[1].Value != "4444" {
		t.Fatalf("second condition = %#v, want port 4444", got.Conditions.Children[1])
	}
}

func TestParseVisualBlocksLLMContentAdjustsUnlinkBlock(t *testing.T) {
	content := `{"trigger":"unlink","action":"BLOCK","conditions":{"id":"root","type":"AND","children":[{"type":"CONDITION","field":"comm","operator":"==","value":"rm"}]},"mapMode":"NONE","mapKey":"pid","mapLimit":10}`

	got, err := parseVisualBlocksLLMContent(content)
	if err != nil {
		t.Fatalf("parseVisualBlocksLLMContent() error = %v", err)
	}
	if got.Action != "ALERT" {
		t.Fatalf("action = %s, want ALERT", got.Action)
	}
	if len(got.Warnings) == 0 || !strings.Contains(got.Warnings[0], "unlink") {
		t.Fatalf("warnings = %#v, want unlink warning", got.Warnings)
	}
}

func TestParseVisualBlocksLLMContentRejectsSocketFieldOnProcess(t *testing.T) {
	content := `{"trigger":"process","action":"BLOCK","conditions":{"id":"root","type":"AND","children":[{"type":"CONDITION","field":"port","operator":"==","value":"4444"}]},"mapMode":"NONE","mapKey":"pid","mapLimit":10}`

	_, err := parseVisualBlocksLLMContent(content)
	if err == nil || !strings.Contains(err.Error(), "port/ipv4") {
		t.Fatalf("parseVisualBlocksLLMContent() error = %v, want port/ipv4 validation error", err)
	}
}

func TestHandlePluginVisualLLMCompileRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/plugins/visual/llm-compile", handlePluginVisualLLMCompile)
	body := append([]byte(`{"prompt":"`), bytes.Repeat([]byte("x"), int(maxVisualLLMRequestBytes))...)
	body = append(body, []byte(`"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/plugins/visual/llm-compile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
