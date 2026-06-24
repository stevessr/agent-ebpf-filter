package app

import (
	netcore "agent-ebpf-filter/internal/network"
	"net"
)

// ---- moved from backend/zz_merged_backend.go section scope_network.go ----

// ── IP Address Classification (from rustnet bogon.rs) ─────────────────

type IPScope = netcore.IPScope

const (
	ScopePublic        = netcore.ScopePublic
	ScopeLoopback      = netcore.ScopeLoopback
	ScopePrivate       = netcore.ScopePrivate
	ScopeLinkLocal     = netcore.ScopeLinkLocal
	ScopeCGNAT         = netcore.ScopeCGNAT
	ScopeMulticast     = netcore.ScopeMulticast
	ScopeBroadcast     = netcore.ScopeBroadcast
	ScopeDocumentation = netcore.ScopeDocumentation
	ScopeBenchmarking  = netcore.ScopeBenchmarking
	ScopeUnspecified   = netcore.ScopeUnspecified
	ScopeReserved      = netcore.ScopeReserved
	ScopeUniqueLocal   = netcore.ScopeUniqueLocal
	ScopeDiscard       = netcore.ScopeDiscard
	ScopeIPv4Mapped    = netcore.ScopeIPv4Mapped
	ScopeUnknown       = netcore.ScopeUnknown
)

func classifyIPScope(ip net.IP) IPScope {
	return netcore.ClassifyIPScope(ip)
}

func ipScopeIsSuspicious(scope IPScope) bool {
	return netcore.IPScopeIsSuspicious(scope)
}

func ipScopeRiskScore(scope IPScope) float64 {
	return netcore.IPScopeRiskScore(scope)
}
