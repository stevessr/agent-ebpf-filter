package network

import (
	netcore "agent-ebpf-filter/internal/network"
	"net"
)

// ---- moved from backend/zz_merged_backend.go section scope_network.go ----

// ── IP Address Classification (from rustnet bogon.rs) ─────────────────

type IPScope = netcore.IPScope

func classifyIPScope(ip net.IP) IPScope {
	return netcore.ClassifyIPScope(ip)
}

func ipScopeIsSuspicious(scope IPScope) bool {
	return netcore.IPScopeIsSuspicious(scope)
}

func ipScopeRiskScore(scope IPScope) float64 {
	return netcore.IPScopeRiskScore(scope)
}
