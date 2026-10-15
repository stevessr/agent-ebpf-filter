package handlers

import (
    "net/http"
    "errors"
    "context"
    "agent-ebpf-filter/app/jsplugins"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/gin-gonic/gin"
)

func TestPluginInvokeRequiresPolicy(t *testing.T) {
    gin.SetMode(gin.TestMode)
    router := gin.New()
    calls := 0
    RegisterPluginRoutes(router.Group("/plugins"), func(c *gin.Context) {
        calls++
        c.AbortWithStatus(http.StatusForbidden)
    })
    req := httptest.NewRequest(http.MethodPost, "/plugins/valid-plugin/invoke", strings.NewReader(`{"hook":"onEvent","payload":{}}`))
    req.Header.Set("Content-Type","application/json")
    response := httptest.NewRecorder()
    router.ServeHTTP(response, req)
    if response.Code != http.StatusForbidden || calls != 1 {
        t.Fatalf("policy bypass: status=%d calls=%d", response.Code,calls)
    }
}

func TestPluginInvokeDelegates(t *testing.T) {
    gin.SetMode(gin.TestMode)
    withPluginHandlerDeps(t)
    router:=gin.New()
    router.POST("/plugins/:id/invoke",HandleJSInvoke)
    req:=httptest.NewRequest(http.MethodPost,"/plugins/valid-plugin/invoke",strings.NewReader(`{"hook":"onEvent","payload":{"pid":123}}`))
    req.Header.Set("Content-Type","application/json")
    response:=httptest.NewRecorder()
    router.ServeHTTP(response,req)
    if response.Code!=http.StatusOK || !strings.Contains(response.Body.String(),`"ok":true`) {
        t.Fatalf("status=%d body=%s",response.Code,response.Body.String())
    }
}

func TestPluginInvokeErrorStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := withPluginHandlerDeps(t)
	router := gin.New()
	router.POST("/plugins/:id/invoke", HandleJSInvoke)
	testCases := []struct {
		err error
		status int
	}{
		{jsplugins.ErrExecutorBusy, http.StatusTooManyRequests},
		{context.DeadlineExceeded, http.StatusGatewayTimeout},
		{context.Canceled, http.StatusRequestTimeout},
		{errors.New("JavaScript exception"), http.StatusUnprocessableEntity},
	}
	for _, tc := range testCases {
		fake.runJSErr = tc.err
		request := httptest.NewRequest(http.MethodPost, "/plugins/valid-plugin/invoke", strings.NewReader(`{"hook":"onEvent","payload":{}}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("error %v: got HTTP %d, want %d (%s)", tc.err, response.Code, tc.status, response.Body.String())
		}
	}
}
