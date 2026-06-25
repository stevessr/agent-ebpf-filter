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

// startDNSCacheGC launches a background goroutine that evicts expired entries.
func startDNSCacheGC(cache *dnsCache) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			cache.EvictExpired()
		}
	}()
}

// recordDNSQueryFromEvent records a detected DNS query domain for later correlation.
func recordDNSQueryFromEvent(cache *dnsCache, domain string) {
	if domain == "" {
		return
	}
	// Domain names from eBPF may be raw; perform basic validation
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" || len(domain) > 253 {
		return
	}
	cache.Record(domain, "") // IP will be filled from DNS response
}

// correlateDNSResponse correlates a DNS response with the query
func correlateDNSResponse(cache *dnsCache, srcIP string, rawData []byte) {
	netcore.CorrelateDNSResponse(cache, rawData)
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
