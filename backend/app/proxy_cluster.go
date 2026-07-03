package app

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section proxy_cluster.go ----

func clusterTargetFromContext(c *gin.Context) string {
	if target := strings.TrimSpace(c.GetHeader(clusterTargetHeader)); target != "" {
		return target
	}
	if target := strings.TrimSpace(c.Query("cluster")); target != "" {
		return target
	}
	return clusterTargetLocal
}

func shouldProxyPath(path string) bool {
	switch {
	case path == "/ws", path == "/ws/system", path == "/ws/shell", path == "/ws/shell-sessions", path == "/ws/ml-status", path == "/ws/envelopes", path == "/ws/events/graph":
		return true
	case path == "/events/recent", path == "/events/graph", path == "/events/recording", path == "/events/recording/start", path == "/events/recording/stop", path == "/events/recording/replay", path == "/events/recording/browser/save", path == "/metrics":
		return true
	case strings.HasPrefix(path, "/agentsight/"):
		return true
	case strings.HasPrefix(path, "/research/"):
		return true
	case strings.HasPrefix(path, "/api/events") || strings.HasPrefix(path, "/api/runners") || strings.HasPrefix(path, "/api/stream"):
		return true
	case strings.HasPrefix(path, "/config/"):
		return true
	case strings.HasPrefix(path, "/system/"):
		return true
	case strings.HasPrefix(path, "/api/v1/"):
		return true
	case strings.HasPrefix(path, "/shell-sessions"):
		return true
	case path == "/register", path == "/unregister", path == "/hooks/event", path == "/mcp":
		return true
	default:
		return false
	}
}

func isProtectedClusterProxyPath(path string) bool {
	switch {
	case path == "/ws", path == "/ws/system", path == "/ws/shell", path == "/ws/shell-sessions", path == "/ws/ml-status", path == "/ws/envelopes", path == "/ws/events/graph":
		return true
	case path == "/events/recent", path == "/events/graph", path == "/events/recording", path == "/events/recording/start", path == "/events/recording/stop", path == "/events/recording/replay", path == "/events/recording/browser/save", path == "/metrics":
		return true
	case strings.HasPrefix(path, "/agentsight/"):
		return true
	case strings.HasPrefix(path, "/research/"):
		return true
	case strings.HasPrefix(path, "/api/events") || strings.HasPrefix(path, "/api/runners") || strings.HasPrefix(path, "/api/stream"):
		return true
	case strings.HasPrefix(path, "/config/"):
		return true
	case strings.HasPrefix(path, "/system/"):
		return true
	case strings.HasPrefix(path, "/api/v1/"):
		return true
	case strings.HasPrefix(path, "/shell-sessions"):
		return true
	case path == "/register", path == "/unregister", path == "/hooks/event":
		return true
	case path == "/mcp":
		return true
	default:
		return false
	}
}

func clusterProxyRequestAllowed(c *gin.Context) bool {
	if gin.Mode() != gin.ReleaseMode || os.Getenv("DISABLE_AUTH") == "true" {
		return true
	}
	if clusterRequestAuthAllowed(c) {
		return true
	}
	if !isProtectedClusterProxyPath(c.Request.URL.Path) {
		return true
	}
	token := requestAuthToken(c)
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(runtimeSettingsStore.ExpectedToken())) == 1
}

func (m *clusterManager) targetNode(target string) (ClusterNode, bool) {
	if target == "" || target == clusterTargetLocal {
		return ClusterNode{}, false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if target == m.config.NodeID {
		return ClusterNode{}, false
	}
	if node, ok := m.nodes[target]; ok && node != nil {
		return *node, true
	}
	return ClusterNode{}, false
}

func (m *clusterManager) reverseProxyForNode(node ClusterNode) (*httputil.ReverseProxy, error) {
	baseURL := strings.TrimSpace(node.URL)
	if baseURL == "" {
		return nil, fmt.Errorf("cluster node %q has no URL", node.ID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if proxy, ok := m.proxyCache[baseURL]; ok {
		return proxy, nil
	}

	targetURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
		req.Header.Del("X-API-KEY")
		req.Header.Del("Authorization")
		req.Header.Set(clusterProxyHeader, "1")
		m.mu.RLock()
		account := m.config.Account
		password := m.config.Password
		m.mu.RUnlock()
		if account != "" {
			req.Header.Set(clusterAccountHeader, account)
		}
		if password != "" {
			req.Header.Set(clusterPasswordHeader, password)
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[WARN] cluster proxy to %s failed: %v", baseURL, err)
		http.Error(w, fmt.Sprintf("cluster proxy to %s failed: %v", baseURL, err), http.StatusBadGateway)
	}

	m.proxyCache[baseURL] = proxy
	return proxy, nil
}

func (m *clusterManager) proxyRequest(c *gin.Context, target string) bool {
	if !m.IsMaster() {
		return false
	}
	if !shouldProxyPath(c.Request.URL.Path) {
		return false
	}
	if !clusterProxyRequestAllowed(c) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return true
	}

	node, ok := m.targetNode(target)
	if !ok {
		if target == "" || target == clusterTargetLocal {
			return false
		}
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "cluster target not found"})
		return true
	}
	if strings.TrimSpace(node.URL) == "" {
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "cluster target has no URL"})
		return true
	}

	proxy, err := m.reverseProxyForNode(node)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return true
	}

	proxy.ServeHTTP(c.Writer, c.Request)
	c.Abort()
	return true
}

func clusterGatewayMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if clusterManagerStore.proxyRequest(c, clusterTargetFromContext(c)) {
			return
		}
		c.Next()
	}
}

func clusterStateHandler(c *gin.Context) {
	c.JSON(http.StatusOK, clusterManagerStore.StateSnapshot())
}

func clusterNodesHandler(c *gin.Context) {
	c.JSON(http.StatusOK, ClusterNodesResponse{Nodes: clusterManagerStore.SnapshotNodes()})
}

func clusterHeartbeatHandler(c *gin.Context) {
	if !clusterControlAuthAllowed(c) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if clusterManagerStore.IsSlave() {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "slave nodes do not accept cluster heartbeats"})
		return
	}

	var req ClusterHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid cluster heartbeat payload"})
		return
	}

	if req.NodeID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing nodeId"})
		return
	}

	node := clusterManagerStore.upsertHeartbeat(req)
	c.JSON(http.StatusOK, ClusterHeartbeatResponse{
		OK:         true,
		ReceivedAt: time.Now().UTC(),
		Registered: node,
	})
}

func startClusterHeartbeatLoop() {
	cfg := clusterManagerStore.ConfigSnapshot()
	if cfg.Role != ClusterRoleSlave || strings.TrimSpace(cfg.MasterURL) == "" {
		return
	}

	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		ticker := time.NewTicker(clusterHeartbeatEvery)
		defer ticker.Stop()

		send := func() {
			state := clusterManagerStore.StateSnapshot()
			body := ClusterHeartbeatRequest{
				NodeID:   state.NodeID,
				NodeName: state.NodeName,
				NodeURL:  state.NodeURL,
				Role:     state.Role,
				Version:  clusterVersion,
			}
			payload, err := json.Marshal(body)
			if err != nil {
				log.Printf("[WARN] failed to marshal cluster heartbeat: %v", err)
				return
			}

			req, err := http.NewRequest(http.MethodPost, cfg.MasterURL+"/cluster/heartbeat", strings.NewReader(string(payload)))
			if err != nil {
				log.Printf("[WARN] failed to build cluster heartbeat request: %v", err)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(clusterAccountHeader, cfg.Account)
			req.Header.Set(clusterPasswordHeader, cfg.Password)

			resp, err := client.Do(req)
			if err != nil {
				log.Printf("[WARN] cluster heartbeat failed: %v", err)
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode >= 300 {
				log.Printf("[WARN] cluster heartbeat returned %s", resp.Status)
			}
		}

		send()
		for range ticker.C {
			send()
		}
	}()
}
