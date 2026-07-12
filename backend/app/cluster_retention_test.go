package app

import (
	"strings"
	"testing"
	"time"
)

func TestClusterManagerPrunesStaleNodesAndProxyEntries(t *testing.T) {
	manager := newClusterManager(ClusterConfig{Role: ClusterRoleMaster, NodeID: "local"})
	manager.nodeRetention = time.Minute
	manager.proxyRetention = time.Hour
	node := manager.upsertHeartbeat(ClusterHeartbeatRequest{NodeID: "stale", NodeURL: "http://127.0.0.1:9001", Role: ClusterRoleSlave})
	if _, err := manager.reverseProxyForNode(node); err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	manager.mu.Lock()
	manager.nodes[node.ID].LastSeen = time.Now().UTC().Add(-2 * time.Minute)
	manager.mu.Unlock()

	nodes := manager.SnapshotNodes()
	if len(nodes) != 1 || !nodes[0].IsLocal {
		t.Fatalf("expected only local node after stale cleanup, got %#v", nodes)
	}
	manager.mu.RLock()
	_, cached := manager.proxyCache[node.ID]
	manager.mu.RUnlock()
	if cached {
		t.Fatal("proxy cache entry survived removal of its stale node")
	}
}

func TestValidateClusterHeartbeatRequestBoundsFields(t *testing.T) {
	t.Parallel()
	valid := ClusterHeartbeatRequest{NodeID: "worker", NodeName: "worker", NodeURL: "https://127.0.0.1:9000", Role: ClusterRoleSlave, Version: "1"}
	if err := validateClusterHeartbeatRequest(&valid); err != nil {
		t.Fatalf("valid heartbeat rejected: %v", err)
	}

	tests := []ClusterHeartbeatRequest{
		{NodeID: strings.Repeat("x", clusterNodeIDMaxBytes+1)},
		{NodeID: "worker", NodeName: strings.Repeat("x", clusterNodeNameMaxBytes+1)},
		{NodeID: "worker", NodeURL: strings.Repeat("x", clusterNodeURLMaxBytes+1)},
		{NodeID: "worker", Version: strings.Repeat("x", clusterVersionMaxBytes+1)},
		{NodeID: "worker", Role: ClusterRole("invalid")},
		{NodeID: "worker", NodeURL: "file:///tmp/node"},
	}
	for _, req := range tests {
		if err := validateClusterHeartbeatRequest(&req); err == nil {
			t.Fatalf("invalid heartbeat accepted: %#v", req)
		}
	}

	normalized := ClusterHeartbeatRequest{
		NodeID:   "  worker  ",
		NodeName: "  Worker One  ",
		NodeURL:  "  https://127.0.0.1:9000  ",
		Role:     ClusterRole("  slave  "),
		Version:  "  1.2.3  ",
	}
	if err := validateClusterHeartbeatRequest(&normalized); err != nil {
		t.Fatalf("normalizable heartbeat rejected: %v", err)
	}
	if normalized.NodeID != "worker" || normalized.NodeName != "Worker One" || normalized.NodeURL != "https://127.0.0.1:9000" || normalized.Role != ClusterRoleSlave || normalized.Version != "1.2.3" {
		t.Fatalf("heartbeat fields were not normalized: %#v", normalized)
	}
}

func TestClusterManagerCapsRemoteNodes(t *testing.T) {
	manager := newClusterManager(ClusterConfig{Role: ClusterRoleMaster, NodeID: "local"})
	manager.maxRemoteNodes = 2
	manager.nodeRetention = time.Hour

	nodeA := manager.upsertHeartbeat(ClusterHeartbeatRequest{NodeID: "a", NodeURL: "http://127.0.0.1:9101", Role: ClusterRoleSlave})
	nodeB := manager.upsertHeartbeat(ClusterHeartbeatRequest{NodeID: "b", NodeURL: "http://127.0.0.1:9102", Role: ClusterRoleSlave})
	manager.mu.Lock()
	manager.nodes[nodeA.ID].LastSeen = time.Now().UTC().Add(-time.Minute)
	manager.mu.Unlock()
	nodeC := manager.upsertHeartbeat(ClusterHeartbeatRequest{NodeID: "c", NodeURL: "http://127.0.0.1:9103", Role: ClusterRoleSlave})

	if _, ok := manager.targetNode(nodeA.ID); ok {
		t.Fatal("oldest node survived hard cap")
	}
	if _, ok := manager.targetNode(nodeB.ID); !ok {
		t.Fatal("recent node b was evicted")
	}
	if _, ok := manager.targetNode(nodeC.ID); !ok {
		t.Fatal("recent node c was evicted")
	}
}

func TestClusterManagerCapsProxyCache(t *testing.T) {
	manager := newClusterManager(ClusterConfig{Role: ClusterRoleMaster, NodeID: "local"})
	manager.maxRemoteNodes = 3
	manager.maxProxyEntries = 2
	manager.nodeRetention = time.Hour
	manager.proxyRetention = time.Hour

	nodes := []ClusterNode{
		manager.upsertHeartbeat(ClusterHeartbeatRequest{NodeID: "a", NodeURL: "http://127.0.0.1:9111", Role: ClusterRoleSlave}),
		manager.upsertHeartbeat(ClusterHeartbeatRequest{NodeID: "b", NodeURL: "http://127.0.0.1:9112", Role: ClusterRoleSlave}),
		manager.upsertHeartbeat(ClusterHeartbeatRequest{NodeID: "c", NodeURL: "http://127.0.0.1:9113", Role: ClusterRoleSlave}),
	}
	for _, node := range nodes[:2] {
		if _, err := manager.reverseProxyForNode(node); err != nil {
			t.Fatalf("create proxy for %s: %v", node.ID, err)
		}
	}
	manager.mu.Lock()
	manager.proxyCache[nodes[0].ID].lastUsed = time.Now().UTC().Add(-time.Minute)
	manager.mu.Unlock()
	if _, err := manager.reverseProxyForNode(nodes[2]); err != nil {
		t.Fatalf("create proxy for %s: %v", nodes[2].ID, err)
	}

	manager.mu.RLock()
	_, oldestCached := manager.proxyCache[nodes[0].ID]
	proxyCount := len(manager.proxyCache)
	manager.mu.RUnlock()
	if oldestCached || proxyCount != 2 {
		t.Fatalf("proxy cap did not evict oldest entry: oldestCached=%t count=%d", oldestCached, proxyCount)
	}
}

func TestClusterManagerReplacesProxyWhenNodeURLChanges(t *testing.T) {
	manager := newClusterManager(ClusterConfig{Role: ClusterRoleMaster, NodeID: "local"})
	node := manager.upsertHeartbeat(ClusterHeartbeatRequest{NodeID: "worker", NodeURL: "http://127.0.0.1:9201", Role: ClusterRoleSlave})
	first, err := manager.reverseProxyForNode(node)
	if err != nil {
		t.Fatalf("create first proxy: %v", err)
	}

	node = manager.upsertHeartbeat(ClusterHeartbeatRequest{NodeID: "worker", NodeURL: "http://127.0.0.1:9202", Role: ClusterRoleSlave})
	manager.mu.RLock()
	_, cachedBeforeRecreate := manager.proxyCache[node.ID]
	manager.mu.RUnlock()
	if cachedBeforeRecreate {
		t.Fatal("URL change did not invalidate prior proxy")
	}
	second, err := manager.reverseProxyForNode(node)
	if err != nil {
		t.Fatalf("create replacement proxy: %v", err)
	}
	if first == second {
		t.Fatal("URL change reused stale reverse proxy")
	}
	manager.mu.RLock()
	entry := manager.proxyCache[node.ID]
	manager.mu.RUnlock()
	if entry == nil || entry.url != node.URL {
		t.Fatalf("replacement proxy has wrong URL: %#v", entry)
	}
}

func TestClusterManagerRejectsUnsafeProxyURLs(t *testing.T) {
	manager := newClusterManager(ClusterConfig{Role: ClusterRoleMaster, NodeID: "local"})
	tests := []struct {
		name    string
		rawURL  string
		wantErr string
	}{
		{name: "file scheme", rawURL: "file:///tmp/socket", wantErr: "http or https"},
		{name: "missing host", rawURL: "http:///api", wantErr: "include a host"},
		{name: "userinfo", rawURL: "https://user:secret@example.com", wantErr: "must not include credentials"},
		{name: "relative", rawURL: "/local/path", wantErr: "http or https"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := manager.reverseProxyForNode(ClusterNode{ID: test.name, URL: test.rawURL})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
			}
		})
	}
	manager.mu.RLock()
	cacheLen := len(manager.proxyCache)
	manager.mu.RUnlock()
	if cacheLen != 0 {
		t.Fatalf("unsafe URLs populated proxy cache: %d entries", cacheLen)
	}
	if _, err := manager.reverseProxyForNode(ClusterNode{ID: "valid", URL: "HTTPS://example.com"}); err != nil {
		t.Fatalf("valid case-insensitive HTTPS URL was rejected: %v", err)
	}
}
