package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/internal/geoip"
	netcore "agent-ebpf-filter/internal/network"

	"github.com/gin-gonic/gin"
	gnet "github.com/shirou/gopsutil/v3/net"
)

// ---- moved from app/enrichment_handlers.go ----

// HandleNetworkFlows returns aggregated network flow view.
func HandleNetworkFlows(c *gin.Context) {
	query := netcore.FlowQuery{
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
	result := Deps.NetworkFlowAggregator.Query(query)
	c.JSON(http.StatusOK, result)
}

// HandleNetworkFlowByID returns one flow by stable flow ID.
func HandleNetworkFlowByID(c *gin.Context) {
	flowID := strings.TrimSpace(c.Param("flowID"))
	flow, ok := Deps.NetworkFlowAggregator.Get(flowID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "flow not found"})
		return
	}
	c.JSON(http.StatusOK, flow)
}

// HandleTCPState returns TCP connection state tracking.
func HandleTCPState(c *gin.Context) {
	conns := Deps.TCPTracker.Snapshot()

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
			Key:        netcore.TCPConnKey(conn.SrcIP, conn.DstIP, conn.SrcPort, conn.DstPort),
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

// HandleNetworkAnalyze analyzes an endpoint for enrichment info.
func HandleNetworkAnalyze(c *gin.Context) {
	endpoint := strings.TrimSpace(c.Query("endpoint"))
	if endpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "endpoint query parameter required"})
		return
	}

	scope, service, domain, risk := Deps.AnalyzeEndpoint(endpoint)

	c.JSON(http.StatusOK, gin.H{
		"endpoint":     endpoint,
		"ipScope":      string(scope),
		"service":      service,
		"domain":       domain,
		"riskScore":    risk,
		"isSuspicious": netcore.IPScopeIsSuspicious(scope) || netcore.IsSuspiciousPortService(service),
	})
}

// HandleGeoIPLookup performs GeoIP lookup for an IP.
func HandleGeoIPLookup(c *gin.Context) {
	ip := strings.TrimSpace(c.Query("ip"))
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip query parameter required"})
		return
	}

	record, ok := Deps.GeoIPDB.Lookup(ip)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	scope, service, domain, risk := Deps.AnalyzeEndpoint(ip)

	c.JSON(http.StatusOK, gin.H{
		"ip":          ip,
		"country":     record.Country,
		"countryCode": record.CountryCode,
		"asnOrg":      record.ASNOrg,
		"ipScope":     string(scope),
		"service":     service,
		"domain":      domain,
		"riskScore":   risk,
		"isHighRisk":  geoipIsHighRiskCountry(record.CountryCode),
	})
}

// HandleDNSLookup checks DNS cache for an IP.
func HandleDNSLookup(c *gin.Context) {
	ip := strings.TrimSpace(c.Query("ip"))
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip query parameter required"})
		return
	}

	domain, found := Deps.DNSCorrelation.LookupIP(ip)
	reverse := ""
	if domain != "" {
		if revIP, ok := Deps.DNSCorrelation.LookupDomain(domain); ok {
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

// HandleDNSCache dumps active DNS cache entries.
func HandleDNSCache(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"entries": Deps.DNSCorrelation.Snapshot(),
	})
}

// HandleNetworkInterfaces returns interface counters for flow workspace.
func HandleNetworkInterfaces(c *gin.Context) {
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

// HandleNetworkFlowJSONLExport exports current flow snapshot as metadata-only JSONL.
func HandleNetworkFlowJSONLExport(c *gin.Context) {
	query := netcore.FlowQuery{
		Filter:       strings.TrimSpace(c.Query("filter")),
		Sort:         strings.TrimSpace(c.Query("sort")),
		ShowHistoric: parseBoolQuery(c.Query("showHistoric")),
		Limit:        500,
	}
	result := Deps.NetworkFlowAggregator.Query(query)
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Content-Disposition", `attachment; filename="network-flows.jsonl"`)
	enc := json.NewEncoder(c.Writer)
	for _, flow := range result.Flows {
		if err := enc.Encode(flow); err != nil {
			return
		}
	}
}

// ── helpers ──────────────────────────────────────────────────────────

func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func geoipIsHighRiskCountry(countryCode string) bool {
	return geoip.IsHighRiskCountry(countryCode)
}
