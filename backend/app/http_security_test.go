package app

import (
	"bytes"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestBackendListenHostSecurityMatrix(t *testing.T) {
	for _, tc := range []struct {
		host, disable, want string
	}{
		{"", "", "127.0.0.1"},
		{"", "true", "127.0.0.1"},
		{"127.0.0.2", "true", "127.0.0.2"},
		{"::1", "true", "::1"},
		{"0.0.0.0", "false", "0.0.0.0"},
		{"::", "", "::"},
		{"0.0.0.0", "true", ""},
		{"::", "true", ""},
		{"192.0.2.10", "true", ""},
		{"localhost", "true", ""},
		{"127.0.0.1:8080", "", ""},
	} {
		t.Run(tc.host+"/disable="+tc.disable, func(t *testing.T) {
			t.Setenv("AGENT_BACKEND_HOST", tc.host)
			t.Setenv("DISABLE_AUTH", tc.disable)
			got, err := backendListenHost()
			if tc.want == "" {
				if err == nil {
					t.Fatal("unsafe or invalid address accepted")
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("host=%q err=%v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestListenBackendRejectsUnsafeBindBeforeListen(t *testing.T) {
	t.Setenv("AGENT_DESKTOP_API_SOCKET", "")
	t.Setenv("AGENT_BACKEND_HOST", "0.0.0.0")
	t.Setenv("DISABLE_AUTH", "true")
	old := listenTCP
	listenTCP = func(_, _ string) (net.Listener, error) {
		t.Fatal("unsafe configuration reached net.Listen")
		return nil, nil
	}
	t.Cleanup(func() { listenTCP = old })
	if _, _, err := listenBackend(); err == nil {
		t.Fatal("expected unsafe bind error")
	}
}

func TestListenBackendExplicitIPv6Address(t *testing.T) {
	t.Setenv("AGENT_DESKTOP_API_SOCKET", "")
	t.Setenv("AGENT_BACKEND_HOST", "::1")
	t.Setenv("AGENT_BACKEND_PORT", "18443")
	t.Setenv("DISABLE_AUTH", "true")
	old := listenTCP
	listenTCP = func(network, address string) (net.Listener, error) {
		if network != "tcp" || address != "[::1]:18443" {
			t.Fatalf("unexpected bind %s %s", network, address)
		}
		return newTestListener(), nil
	}
	t.Cleanup(func() { listenTCP = old })
	l, _, err := listenBackend()
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Close()
}

func TestWebSocketOriginPolicy(t *testing.T) {
	t.Setenv("AGENT_WS_ALLOWED_ORIGINS", "https://ui.example:9443, http://localhost:5173, *, https://bad.example/path")
	for _, tc := range []struct {
		name    string
		origins []string
		want    bool
	}{
		{"native", nil, true},
		{"same host", []string{"http://127.0.0.1:8080"}, true},
		{"TLS proxy preserves Host", []string{"https://127.0.0.1:8080"}, true},
		{"allowed dev", []string{"http://localhost:5173"}, true},
		{"allowed UI", []string{"https://ui.example:9443"}, true},
		{"wrong scheme", []string{"http://ui.example:9443"}, false},
		{"wrong port", []string{"https://ui.example:9444"}, false},
		{"suffix spoof", []string{"https://ui.example.attacker.test:9443"}, false},
		{"foreign", []string{"https://other.example"}, false},
		{"null", []string{"null"}, false},
		{"empty header", []string{""}, false},
		{"userinfo", []string{"http://user@127.0.0.1:8080"}, false},
		{"path", []string{"http://127.0.0.1:8080/"}, false},
		{"query", []string{"http://127.0.0.1:8080?key=x"}, false},
		{"fragment", []string{"http://127.0.0.1:8080#"}, false},
		{"duplicate headers", []string{"http://127.0.0.1:8080", "https://other.example"}, false},
		{"invalid whitelist entry", []string{"https://bad.example"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/ws", nil)
			for _, origin := range tc.origins {
				req.Header.Add("Origin", origin)
			}
			req.Header.Set("X-Forwarded-Host", "other.example")
			if got := allowWebSocketOrigin(req); got != tc.want {
				t.Fatalf("allowed=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestHTTPRouterDoesNotLogCredentials(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	var output bytes.Buffer
	oldWriter, oldErrorWriter, oldLog := gin.DefaultWriter, gin.DefaultErrorWriter, log.Writer()
	gin.DefaultWriter, gin.DefaultErrorWriter = &output, &output
	log.SetOutput(&output)
	t.Cleanup(func() {
		gin.DefaultWriter, gin.DefaultErrorWriter = oldWriter, oldErrorWriter
		log.SetOutput(oldLog)
	})
	r := newHTTPRouter()
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/fail", func(c *gin.Context) {
		_ = c.Error(errors.New("ERROR_SECRET"))
		c.Status(http.StatusUnauthorized)
	})
	r.GET("/panic", func(c *gin.Context) { panic("PANIC_SECRET") })
	for path, want := range map[string]int{"/ok": 200, "/fail": 401, "/panic": 500} {
		req := httptest.NewRequest(http.MethodGet, path+"?key=QUERY_SECRET", nil)
		req.Header.Set("X-API-KEY", "HEADER_SECRET")
		req.Header.Set("Authorization", "Bearer BEARER_SECRET")
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s returned %d, want %d", path, res.Code, want)
		}
		if !strings.Contains(output.String(), path) {
			t.Fatalf("missing path %s in access log", path)
		}
	}
	for _, secret := range []string{"QUERY_SECRET", "HEADER_SECRET", "BEARER_SECRET", "ERROR_SECRET", "PANIC_SECRET"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("credential leaked to log: %s", secret)
		}
	}
}

func TestWebSocketHandshakeRequiresAuthAndAllowedOrigin(t *testing.T) {
	t.Setenv("DISABLE_AUTH", "false")
	t.Setenv("AGENT_WS_ALLOWED_ORIGINS", "https://ui.example")
	oldMode, oldStore := gin.Mode(), runtimeSettingsStore
	gin.SetMode(gin.DebugMode)
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{AccessToken: "ws-test-token"}}
	t.Cleanup(func() { gin.SetMode(oldMode); runtimeSettingsStore = oldStore })
	r := newHTTPRouter()
	r.GET("/ws", authMiddleware(), func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err == nil {
			_ = conn.Close()
		}
	})
	// Represents a proxy endpoint before the local upgrader. Global middleware
	// must reject foreign origins here too, even with a valid credential.
	r.GET("/proxy", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	server := httptest.NewServer(r)
	defer server.Close()
	for _, tc := range []struct {
		name, origin, token string
		want                int
	}{
		{"native", "", "ws-test-token", http.StatusSwitchingProtocols},
		{"same host", server.URL, "ws-test-token", http.StatusSwitchingProtocols},
		{"explicit origin", "https://ui.example", "ws-test-token", http.StatusSwitchingProtocols},
		{"foreign", "https://other.example", "ws-test-token", http.StatusForbidden},
		{"missing token", server.URL, "", http.StatusUnauthorized},
		{"wrong token", server.URL, "wrong-token", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{}
			if tc.origin != "" {
				headers.Set("Origin", tc.origin)
			}
			if tc.token != "" {
				headers.Set("X-API-KEY", tc.token)
			}
			conn, res, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", headers)
			if conn != nil {
				_ = conn.Close()
			}
			if res != nil {
				defer res.Body.Close()
			}
			if res == nil || res.StatusCode != tc.want {
				t.Fatalf("response=%v err=%v, want %d", res, err, tc.want)
			}
		})
	}
	req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Origin", "https://other.example")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("proxy bypass returned %d", res.Code)
	}
}

func TestAuthenticationIndependentOfGinMode(t *testing.T) {
	oldMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(oldMode) })
	for _, mode := range []string{gin.DebugMode, gin.ReleaseMode, gin.TestMode} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(mode)
			t.Setenv("DISABLE_AUTH", "")
			if !authEnforced() {
				t.Fatal("auth disabled by Gin mode")
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/events/recent", nil)
			if clusterProxyRequestAllowed(c) {
				t.Fatal("cluster proxy bypassed auth")
			}
			r := gin.New()
			r.POST("/hooks/event", hookIngressAuthMiddleware(), func(c *gin.Context) { c.Status(204) })
			res := httptest.NewRecorder()
			r.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/hooks/event", nil))
			if res.Code != http.StatusUnauthorized {
				t.Fatalf("hook auth status=%d", res.Code)
			}
			t.Setenv("DISABLE_AUTH", "true")
			if authEnforced() {
				t.Fatal("explicit development opt-out ignored")
			}
		})
	}
}
