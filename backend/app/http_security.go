package app

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Only literal addresses are accepted so the no-auth check cannot be invalidated
// by a hostname resolving to a different address between validation and bind.
func backendListenHost() (string, error) {
	host := strings.TrimSpace(os.Getenv("AGENT_BACKEND_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("AGENT_BACKEND_HOST must be a literal IPv4 or IPv6 address")
	}
	if !authEnforced() && !ip.IsLoopback() {
		return "", fmt.Errorf("DISABLE_AUTH=true requires a loopback AGENT_BACKEND_HOST")
	}
	return host, nil
}

func parseWebSocketOrigin(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return nil, false
	}
	return u, true
}

func allowWebSocketOrigin(r *http.Request) bool {
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		// Native clients do not send Origin; authentication still applies.
		return true
	}
	if len(origins) != 1 {
		return false
	}
	origin, ok := parseWebSocketOrigin(origins[0])
	if !ok {
		return false
	}
	// Match Gorilla's same-host policy. Do not trust X-Forwarded-Host from
	// arbitrary clients. Reverse proxies must preserve the public Host.
	if strings.EqualFold(origin.Host, r.Host) {
		return true
	}
	for _, raw := range strings.Split(os.Getenv("AGENT_WS_ALLOWED_ORIGINS"), ",") {
		allowed, valid := parseWebSocketOrigin(strings.TrimSpace(raw))
		if valid && origin.Scheme == allowed.Scheme && strings.EqualFold(origin.Host, allowed.Host) {
			return true
		}
	}
	return false
}

func newHTTPRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipQueryString: true,
		// Do not append c.Errors: handler errors may contain URLs or credentials.
		Formatter: func(p gin.LogFormatterParams) string {
			return fmt.Sprintf("[HTTP] %d | %s | %s %s\n", p.StatusCode, p.Latency, p.Method, p.Path)
		},
	}))
	// Gin's default recovery dumps request headers and the panic value. Keep
	// the stack for diagnostics but never log request data or a panic payload.
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		log.Printf("[ERROR] HTTP handler panic\n%s", debug.Stack())
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	r.Use(func(c *gin.Context) {
		// Check before cluster proxying as well as in the local upgrader.
		if websocket.IsWebSocketUpgrade(c.Request) && !allowWebSocketOrigin(c.Request) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "WebSocket origin not allowed"})
			return
		}
		c.Next()
	})
	return r
}
