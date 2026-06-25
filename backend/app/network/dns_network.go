package network

import (
	netcore "agent-ebpf-filter/internal/network"
	"strings"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section dns_network.go ----

type dnsCache = netcore.DNSCache

type dnsCacheSnapshotEntry = netcore.DNSCacheSnapshotEntry

func newDNSCache() *dnsCache {
	return netcore.NewDNSCache()
}

var dnsCorrelation = newDNSCache()

func startDNSCacheGC() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			dnsCorrelation.EvictExpired()
		}
	}()
}

// Process a detected DNS query and record the domain
func recordDNSQueryFromEvent(domain string) {
	if domain == "" {
		return
	}
	// Domain names from eBPF may be raw; perform basic validation
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" || len(domain) > 253 {
		return
	}
	dnsCorrelation.Record(domain, "") // IP will be filled from DNS response
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
