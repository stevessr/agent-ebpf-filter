package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// GET /network/flows - return aggregated network flow view
func handleNetworkFlows(c *gin.Context) {
	flows := networkFlowAggregator.Snapshot()

	// Apply optional filters
	filterPID := strings.TrimSpace(c.Query("pid"))
	filterDomain := strings.TrimSpace(c.Query("domain"))
	filterService := strings.TrimSpace(c.Query("service"))
	filterScope := strings.TrimSpace(c.Query("scope"))

	filtered := make([]NetworkFlowSummary, 0, len(flows))
	for _, flow := range flows {
		if filterPID != "" {
			pid, err := strconv.Atoi(filterPID)
			if err != nil {
				continue
			}
			found := false
			for _, p := range flow.ProcessPIDs {
				if p == uint32(pid) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if filterDomain != "" && !strings.Contains(strings.ToLower(flow.DstDomain), strings.ToLower(filterDomain)) {
			continue
		}
		if filterService != "" && !strings.EqualFold(flow.DstService, filterService) {
			continue
		}
		if filterScope != "" && !strings.EqualFold(flow.IPScope, filterScope) {
			continue
		}
		filtered = append(filtered, flow)
	}

	c.JSON(http.StatusOK, gin.H{
		"flows": filtered,
		"total": len(filtered),
	})
}

// GET /network/tcp-state - return TCP connection state tracking
func handleTCPState(c *gin.Context) {
	conns := tcpTracker.Snapshot()

	type tcpStateResponse struct {
		Key        string `json:"key"`
		SrcIP      string `json:"srcIp"`
		DstIP      string `json:"dstIp"`
		SrcPort    uint32 `json:"srcPort"`
		DstPort    uint32 `json:"dstPort"`
		State      string `json:"state"`
		PID        uint32 `json:"pid"`
		Comm       string `json:"comm"`
		LastUpdate int64  `json:"lastUpdate"`
	}

	items := make([]tcpStateResponse, 0, len(conns))
	for _, conn := range conns {
		items = append(items, tcpStateResponse{
			Key:        tcpTracker.connKey(conn.SrcIP, conn.DstIP, conn.SrcPort, conn.DstPort),
			SrcIP:      conn.SrcIP,
			DstIP:      conn.DstIP,
			SrcPort:    conn.SrcPort,
			DstPort:    conn.DstPort,
			State:      conn.State.String(),
			PID:        conn.PID,
			Comm:       conn.Comm,
			LastUpdate: conn.LastUpdate.UnixMilli(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"connections": items,
		"total":       len(items),
	})
}

// GET /network/analyze - analyze an endpoint for enrichment info
func handleNetworkAnalyze(c *gin.Context) {
	endpoint := strings.TrimSpace(c.Query("endpoint"))
	if endpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "endpoint query parameter required"})
		return
	}

	scope, service, domain, risk := analyzeEndpoint(endpoint)

	c.JSON(http.StatusOK, gin.H{
		"endpoint":    endpoint,
		"ipScope":     string(scope),
		"service":     service,
		"domain":      domain,
		"riskScore":   risk,
		"isSuspicious": ipScopeIsSuspicious(scope) || isSuspiciousPortService(service),
	})
}

// GET /network/dns-lookup - check DNS cache for an IP
func handleDNSLookup(c *gin.Context) {
	ip := strings.TrimSpace(c.Query("ip"))
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip query parameter required"})
		return
	}

	domain, found := dnsCorrelation.LookupIP(ip)
	reverse := ""
	if domain != "" {
		if revIP, ok := dnsCorrelation.LookupDomain(domain); ok {
			reverse = revIP
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"ip":      ip,
		"domain":  domain,
		"found":   found,
		"reverse": reverse,
	})
}
