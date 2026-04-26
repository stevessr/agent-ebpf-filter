package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	clusterTargetLocal    = "local"
	clusterTargetHeader   = "X-Cluster-Target"
	clusterProxyHeader    = "X-Cluster-Proxy"
	clusterAccountHeader  = "X-Cluster-Account"
	clusterPasswordHeader = "X-Cluster-Password"
	clusterHeartbeatEvery = 5 * time.Second
	clusterOfflineAfter   = 15 * time.Second
	clusterVersion        = "1.0.0"
)

type ClusterRole string

const (
	ClusterRoleMaster ClusterRole = "master"
	ClusterRoleSlave  ClusterRole = "slave"
)

type ClusterConfig struct {
	Role      ClusterRole
	MasterURL string
	NodeURL   string
	NodeID    string
	NodeName  string
	Account   string
	Password  string
	Hostname  string
}

type ClusterNode struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	URL      string      `json:"url"`
	Role     ClusterRole `json:"role"`
	Status   string      `json:"status"`
	LastSeen time.Time   `json:"lastSeen"`
	IsLocal  bool        `json:"isLocal"`
	Version  string      `json:"version,omitempty"`
}

type ClusterStateResponse struct {
	Role               ClusterRole `json:"role"`
	MasterURL          string      `json:"masterUrl,omitempty"`
	NodeURL            string      `json:"nodeUrl"`
	NodeID             string      `json:"nodeId"`
	NodeName           string      `json:"nodeName"`
	AccountConfigured  bool        `json:"accountConfigured"`
	PasswordConfigured bool        `json:"passwordConfigured"`
	LocalNode          ClusterNode `json:"localNode"`
}

type ClusterNodesResponse struct {
	Nodes []ClusterNode `json:"nodes"`
}

type ClusterHeartbeatRequest struct {
	NodeID   string      `json:"nodeId"`
	NodeName string      `json:"nodeName"`
	NodeURL  string      `json:"nodeUrl"`
	Role     ClusterRole `json:"role"`
	Version  string      `json:"version,omitempty"`
}

type ClusterHeartbeatResponse struct {
	OK         bool        `json:"ok"`
	ReceivedAt time.Time   `json:"receivedAt"`
	Registered ClusterNode `json:"registered"`
}

type clusterManager struct {
	mu         sync.RWMutex
	config     ClusterConfig
	nodes      map[string]*ClusterNode
	proxyCache map[string]*httputil.ReverseProxy
}

var clusterManagerStore = newClusterManager(loadClusterConfigFromEnv())

func loadClusterConfigFromEnv() ClusterConfig {
	role := ClusterRoleMaster
	masterURL := strings.TrimSpace(os.Getenv("AGENT_CLUSTER_MASTER_URL"))
	account := strings.TrimSpace(os.Getenv("AGENT_CLUSTER_ACCOUNT"))
	password := strings.TrimSpace(os.Getenv("AGENT_CLUSTER_PASSWORD"))
	if masterURL != "" && account != "" && password != "" {
		role = ClusterRoleSlave
	}

	hostname, _ := os.Hostname()

	return ClusterConfig{
		Role:      role,
		MasterURL: normalizeClusterURL(masterURL),
		NodeURL:   normalizeClusterURL(strings.TrimSpace(os.Getenv("AGENT_CLUSTER_NODE_URL"))),
		NodeID:    strings.TrimSpace(os.Getenv("AGENT_CLUSTER_NODE_ID")),
		NodeName:  strings.TrimSpace(os.Getenv("AGENT_CLUSTER_NODE_NAME")),
		Account:   account,
		Password:  password,
		Hostname:  hostname,
	}
}

func normalizeClusterURL(raw string) string {
	raw = strings.TrimSpace(raw)
	return strings.TrimRight(raw, "/")
}

func newClusterManager(config ClusterConfig) *clusterManager {
	return &clusterManager{
		config:     config,
		nodes:      make(map[string]*ClusterNode),
		proxyCache: make(map[string]*httputil.ReverseProxy),
	}
}

func (m *clusterManager) ConfigurePort(port int) {
	if port <= 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.config.NodeURL == "" {
		m.config.NodeURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	}
	if m.config.NodeID == "" {
		host := sanitizeClusterHostname(m.config.Hostname)
		if host == "" {
			host = "node"
		}
		m.config.NodeID = fmt.Sprintf("%s-%d", host, port)
	}
	if m.config.NodeName == "" {
		if m.config.Hostname != "" {
			m.config.NodeName = m.config.Hostname
		} else {
			m.config.NodeName = m.config.NodeID
		}
	}

	m.nodes[m.config.NodeID] = &ClusterNode{
		ID:       m.config.NodeID,
		Name:     m.config.NodeName,
		URL:      m.config.NodeURL,
		Role:     m.config.Role,
		Status:   "online",
		LastSeen: time.Now().UTC(),
		IsLocal:  true,
		Version:  clusterVersion,
	}
}

func sanitizeClusterHostname(host string) string {
	host = strings.TrimSpace(host)
	host = strings.ToLower(host)
	if host == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-_")
}

func (m *clusterManager) ConfigSnapshot() ClusterConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

func (m *clusterManager) Role() ClusterRole {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config.Role == "" {
		return ClusterRoleMaster
	}
	return m.config.Role
}

func (m *clusterManager) IsMaster() bool {
	return m.Role() == ClusterRoleMaster
}

func (m *clusterManager) IsSlave() bool {
	return m.Role() == ClusterRoleSlave
}

func (m *clusterManager) localNodeLocked() ClusterNode {
	return ClusterNode{
		ID:       m.config.NodeID,
		Name:     m.config.NodeName,
		URL:      m.config.NodeURL,
		Role:     m.config.Role,
		Status:   "online",
		LastSeen: time.Now().UTC(),
		IsLocal:  true,
		Version:  clusterVersion,
	}
}

func (m *clusterManager) LocalNode() ClusterNode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.localNodeLocked()
}

func (m *clusterManager) StateSnapshot() ClusterStateResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ClusterStateResponse{
		Role:               m.config.Role,
		MasterURL:          m.config.MasterURL,
		NodeURL:            m.config.NodeURL,
		NodeID:             m.config.NodeID,
		NodeName:           m.config.NodeName,
		AccountConfigured:  strings.TrimSpace(m.config.Account) != "",
		PasswordConfigured: strings.TrimSpace(m.config.Password) != "",
		LocalNode:          m.localNodeLocked(),
	}
}

func (m *clusterManager) SnapshotNodes() []ClusterNode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodes := make([]ClusterNode, 0, len(m.nodes)+1)
	nodes = append(nodes, m.localNodeLocked())
	for id, node := range m.nodes {
		if node == nil || id == m.config.NodeID {
			continue
		}
		snapshot := *node
		if time.Since(snapshot.LastSeen) > clusterOfflineAfter {
			snapshot.Status = "stale"
		} else {
			snapshot.Status = "online"
		}
		nodes = append(nodes, snapshot)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsLocal != nodes[j].IsLocal {
			return nodes[i].IsLocal
		}
		if nodes[i].LastSeen.Equal(nodes[j].LastSeen) {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].LastSeen.After(nodes[j].LastSeen)
	})
	return nodes
}

func (m *clusterManager) upsertHeartbeat(req ClusterHeartbeatRequest) ClusterNode {
	m.mu.Lock()
	defer m.mu.Unlock()

	if req.NodeID == "" {
		req.NodeID = m.config.NodeID
	}
	if req.NodeURL == "" {
		req.NodeURL = m.config.NodeURL
	}
	if req.NodeName == "" {
		req.NodeName = req.NodeID
	}
	node := ClusterNode{
		ID:       req.NodeID,
		Name:     req.NodeName,
		URL:      normalizeClusterURL(req.NodeURL),
		Role:     req.Role,
		Status:   "online",
		LastSeen: time.Now().UTC(),
		IsLocal:  false,
		Version:  req.Version,
	}
	if node.Version == "" {
		node.Version = clusterVersion
	}
	m.nodes[node.ID] = &node
	return node
}

func (m *clusterManager) authMatches(c *gin.Context, requireProxy bool) bool {
	m.mu.RLock()
	account := strings.TrimSpace(m.config.Account)
	password := strings.TrimSpace(m.config.Password)
	m.mu.RUnlock()
	if account == "" || password == "" {
		return false
	}
	if requireProxy && strings.TrimSpace(c.GetHeader(clusterProxyHeader)) == "" {
		return false
	}

	reqAccount := strings.TrimSpace(c.GetHeader(clusterAccountHeader))
	reqPassword := strings.TrimSpace(c.GetHeader(clusterPasswordHeader))
	if reqAccount == "" || reqPassword == "" {
		user, pass, ok := c.Request.BasicAuth()
		if ok {
			reqAccount = strings.TrimSpace(user)
			reqPassword = strings.TrimSpace(pass)
		}
	}
	if reqAccount == "" || reqPassword == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(reqAccount), []byte(account)) == 1 &&
		subtle.ConstantTimeCompare([]byte(reqPassword), []byte(password)) == 1
}

func clusterRequestAuthAllowed(c *gin.Context) bool {
	return clusterManagerStore.authMatches(c, true)
}

func clusterControlAuthAllowed(c *gin.Context) bool {
	return clusterManagerStore.authMatches(c, false)
}

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
	case path == "/ws", path == "/ws/system", path == "/ws/shell":
		return true
	case strings.HasPrefix(path, "/config/"):
		return true
	case strings.HasPrefix(path, "/system/"):
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
	case strings.HasPrefix(path, "/config/"):
		return true
	case strings.HasPrefix(path, "/system/"):
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
