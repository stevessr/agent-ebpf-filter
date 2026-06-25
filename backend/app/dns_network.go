package app

import (
	netcore "agent-ebpf-filter/internal/network"
)

// ---- moved from backend/zz_merged_backend.go section dns_network.go ----

type dnsCache = netcore.DNSCache

type dnsCacheSnapshotEntry = netcore.DNSCacheSnapshotEntry

// Package-level global kept for backward compatibility (used by flow.go init).
// New code should use AppCtx.Network.
func newDNSCache() *dnsCache {
	return netcore.NewDNSCache()
}

var dnsCorrelation = newDNSCache()

func startDNSCacheGC() {
	AppCtx.Network.StartDNSCacheGC()
}

// Process a detected DNS query and record the domain
func recordDNSQueryFromEvent(domain string) {
	AppCtx.Network.RecordDNSQueryFromEvent(domain)
}

// Correlate a DNS response with the query
func correlateDNSResponse(srcIP string, rawData []byte) {
	netcore.CorrelateDNSResponse(dnsCorrelation, rawData)
}

func lookupService(port uint16) string {
	return netcore.LookupService(port)
}

func lookupServiceByPort(port uint32) string {
	return netcore.LookupServiceByPort(port)
}

func isSuspiciousPortService(serviceName string) bool {
	return netcore.IsSuspiciousPortService(serviceName)
}
