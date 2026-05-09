package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	gnet "github.com/shirou/gopsutil/v3/net"
)

// GET /network/flows - return aggregated network flow view
func handleNetworkFlows(c *gin.Context) {
	query := networkFlowQuery{
		Filter:       strings.TrimSpace(c.Query("filter")),
		Sort:         strings.TrimSpace(c.Query("sort")),
		ShowHistoric: parseBoolQuery(c.Query("showHistoric")),
		Cursor:       strings.TrimSpace(c.Query("cursor")),
		Domain:       strings.TrimSpace(c.Query("domain")),
		Service:      strings.TrimSpace(c.Query("service")),
		Scope:        strings.TrimSpace(c.Query("scope")),
	}
	if limit, err := strconv.Atoi(strings.TrimSpace(c.Query("limit"))); err == nil {
		query.Limit = limit
	}
	if pid, err := strconv.ParseUint(strings.TrimSpace(c.Query("pid")), 10, 32); err == nil {
		query.PID = uint32(pid)
	}
	result := networkFlowAggregator.Query(query)
	c.JSON(http.StatusOK, result)
}

// GET /network/flows/:flowID - return one flow by stable flow ID
func handleNetworkFlowByID(c *gin.Context) {
	flowID := strings.TrimSpace(c.Param("flowID"))
	flow, ok := networkFlowAggregator.Get(flowID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "flow not found"})
		return
	}
	c.JSON(http.StatusOK, flow)
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
		"endpoint":     endpoint,
		"ipScope":      string(scope),
		"service":      service,
		"domain":       domain,
		"riskScore":    risk,
		"isSuspicious": ipScopeIsSuspicious(scope) || isSuspiciousPortService(service),
	})
}

// GET /network/geoip - lookup GeoIP for an IP
func handleGeoIPLookup(c *gin.Context) {
	ip := strings.TrimSpace(c.Query("ip"))
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip query parameter required"})
		return
	}

	record, ok := geoipDB.Lookup(ip)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	scope, service, domain, risk := analyzeEndpoint(ip)

	c.JSON(http.StatusOK, gin.H{
		"ip":          ip,
		"country":     record.Country,
		"countryCode": record.CountryCode,
		"asnOrg":      record.ASNOrg,
		"ipScope":     string(scope),
		"service":     service,
		"domain":      domain,
		"riskScore":   risk,
		"isHighRisk":  isHighRiskCountry(record.CountryCode),
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

// GET /network/dns-cache - dump active DNS cache entries
func handleDNSCache(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"entries": dnsCorrelation.Snapshot(),
	})
}

// GET /network/interfaces - return interface counters for flow workspace
func handleNetworkInterfaces(c *gin.Context) {
	counters, err := gnet.IOCounters(true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	now := time.Now().UTC().UnixMilli()
	items := make([]gin.H, 0, len(counters))
	for _, counter := range counters {
		items = append(items, gin.H{
			"name":        counter.Name,
			"bytesRecv":   counter.BytesRecv,
			"bytesSent":   counter.BytesSent,
			"packetsRecv": counter.PacketsRecv,
			"packetsSent": counter.PacketsSent,
			"errin":       counter.Errin,
			"errout":      counter.Errout,
			"dropin":      counter.Dropin,
			"dropout":     counter.Dropout,
			"fifoin":      counter.Fifoin,
			"fifoout":     counter.Fifoout,
			"timestamp":   now,
		})
	}
	c.JSON(http.StatusOK, gin.H{"interfaces": items, "total": len(items)})
}

// GET /network/export/jsonl - export current flow snapshot as metadata-only JSONL
func handleNetworkFlowJSONLExport(c *gin.Context) {
	query := networkFlowQuery{
		Filter:       strings.TrimSpace(c.Query("filter")),
		Sort:         strings.TrimSpace(c.Query("sort")),
		ShowHistoric: parseBoolQuery(c.Query("showHistoric")),
		Limit:        500,
	}
	result := networkFlowAggregator.Query(query)
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Content-Disposition", `attachment; filename="network-flows.jsonl"`)
	enc := json.NewEncoder(c.Writer)
	for _, flow := range result.Flows {
		if err := enc.Encode(flow); err != nil {
			return
		}
	}
}

func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
