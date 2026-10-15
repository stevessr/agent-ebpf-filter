package handlers

import (
    "net/http"
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
