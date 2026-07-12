package app

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneClusterStateLocked(time.Now().UTC())

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

	targetURL, err := parseClusterProxyTarget(node.ID, baseURL)
	if err != nil {
		return nil, err
	}
	cacheID := strings.TrimSpace(node.ID)
	if cacheID == "" {
		cacheID = baseURL
	}
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneClusterStateLocked(now)
	if entry := m.proxyCache[cacheID]; entry != nil && entry.proxy != nil && entry.url == baseURL {
		entry.lastUsed = now
		return entry.proxy, nil
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

	m.proxyCache[cacheID] = &clusterProxyCacheEntry{url: baseURL, proxy: proxy, lastUsed: now}
	m.enforceClusterProxyCapLocked(cacheID)
	return proxy, nil
}

func parseClusterProxyTarget(nodeID, raw string) (*url.URL, error) {
	targetURL, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("cluster node %q URL is invalid: %w", nodeID, err)
	}
	if !strings.EqualFold(targetURL.Scheme, "http") && !strings.EqualFold(targetURL.Scheme, "https") {
		return nil, fmt.Errorf("cluster node %q URL must use http or https", nodeID)
	}
	if targetURL.Host == "" || strings.TrimSpace(targetURL.Hostname()) == "" {
		return nil, fmt.Errorf("cluster node %q URL must include a host", nodeID)
	}
	if targetURL.User != nil {
		return nil, fmt.Errorf("cluster node %q URL must not include credentials", nodeID)
	}
	return targetURL, nil
}

func validateClusterHeartbeatRequest(req *ClusterHeartbeatRequest) error {
	if req == nil {
		return fmt.Errorf("missing cluster heartbeat")
	}
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.NodeName = strings.TrimSpace(req.NodeName)
	req.NodeURL = strings.TrimSpace(req.NodeURL)
	req.Role = ClusterRole(strings.TrimSpace(string(req.Role)))
	req.Version = strings.TrimSpace(req.Version)
	if req.NodeID == "" {
		return fmt.Errorf("missing nodeId")
	}
	for label, value := range map[string]string{
		"nodeId": req.NodeID, "nodeName": req.NodeName, "nodeUrl": req.NodeURL, "version": req.Version,
	} {
		limit := clusterNodeNameMaxBytes
		switch label {
		case "nodeId":
			limit = clusterNodeIDMaxBytes
		case "nodeUrl":
			limit = clusterNodeURLMaxBytes
		case "version":
			limit = clusterVersionMaxBytes
		}
		if len(value) > limit {
			return fmt.Errorf("%s exceeds %d bytes", label, limit)
		}
	}
	if req.Role != "" && req.Role != ClusterRoleMaster && req.Role != ClusterRoleSlave {
		return fmt.Errorf("invalid cluster role %q", req.Role)
	}
	if strings.TrimSpace(req.NodeURL) != "" {
		if _, err := parseClusterProxyTarget(req.NodeID, req.NodeURL); err != nil {
			return err
		}
	}
	return nil
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

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, clusterHeartbeatMaxBodyBytes)
	var req ClusterHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		status := http.StatusBadRequest
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			status = http.StatusRequestEntityTooLarge
		}
		c.AbortWithStatusJSON(status, gin.H{"error": "invalid cluster heartbeat payload"})
		return
	}

	if err := validateClusterHeartbeatRequest(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	node := clusterManagerStore.upsertHeartbeat(req)
	c.JSON(http.StatusOK, ClusterHeartbeatResponse{
		OK:         true,
		ReceivedAt: time.Now().UTC(),
		Registered: node,
	})
}

func runConfiguredClusterHeartbeatLoop(ctx context.Context) {
	cfg := clusterManagerStore.ConfigSnapshot()
	if ctx == nil || cfg.Role != ClusterRoleSlave || strings.TrimSpace(cfg.MasterURL) == "" {
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	runClusterHeartbeatLoop(ctx, clusterManagerStore, cfg, client, clusterHeartbeatEvery)
}

func runClusterHeartbeatLoop(ctx context.Context, manager *clusterManager, cfg ClusterConfig, client *http.Client, interval time.Duration) {
	if ctx == nil || manager == nil || client == nil || interval <= 0 {
		return
	}
	send := func() {
		if err := sendClusterHeartbeat(ctx, manager, cfg, client); err != nil && ctx.Err() == nil {
			log.Printf("[WARN] cluster heartbeat failed: %v", err)
		}
	}
	if ctx.Err() != nil {
		return
	}
	send()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

func sendClusterHeartbeat(ctx context.Context, manager *clusterManager, cfg ClusterConfig, client *http.Client) error {
	state := manager.StateSnapshot()
	body := ClusterHeartbeatRequest{
		NodeID:   state.NodeID,
		NodeName: state.NodeName,
		NodeURL:  state.NodeURL,
		Role:     state.Role,
		Version:  clusterVersion,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal cluster heartbeat: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.MasterURL+"/cluster/heartbeat", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build cluster heartbeat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(clusterAccountHeader, cfg.Account)
	req.Header.Set(clusterPasswordHeader, cfg.Password)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("master returned %s", resp.Status)
	}
	return nil
}
