package main

import (
	"crypto/subtle"
	"fmt"
	"net/http/httputil"
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
