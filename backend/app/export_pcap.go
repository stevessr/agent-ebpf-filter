package app

import (
	"agent-ebpf-filter/app/export"
	"agent-ebpf-filter/app/platform"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

// handlePCAPExport is the HTTP handler for PCAP export.
// The PCAP format writing primitives live in the export subpackage.

func handlePCAPExport(c *gin.Context) {
	flows := networkFlowAggregator.Snapshot()
	tcpConns := tcpTracker.Snapshot()

	exportDir := platform.RuntimeSettingsDir()
	exportPath := filepath.Join(exportDir, fmt.Sprintf("network-export-%s.pcap", time.Now().UTC().Format("20060102-150405")))

	f, err := os.Create(exportPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer f.Close()

	if err := export.WritePCAPHeader(f); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	packetCount := 0

	// Export aggregated flows
	for _, flow := range flows {
		srcIP := flow.SrcIP
		if srcIP == "" || srcIP == "0.0.0.0" {
			srcIP = "10.0.0.1"
		}
		dstIP := flow.DstIP
		if dstIP == "" || dstIP == "0.0.0.0" {
			continue
		}

		frame := export.BuildSyntheticEthernetFrame(srcIP, dstIP, flow.SrcPort, flow.DstPort, flow.Protocol, flow.BytesOut)
		if err := export.WritePCAPPacket(f, time.UnixMilli(flow.FirstSeen), frame); err != nil {
			continue
		}
		packetCount++
	}

	// Export TCP connections
	for _, conn := range tcpConns {
		srcIP := conn.SrcIP
		if srcIP == "" {
			srcIP = "10.0.0.1"
		}
		dstIP := conn.DstIP
		if dstIP == "" {
			continue
		}

		frame := export.BuildSyntheticEthernetFrame(srcIP, dstIP, conn.SrcPort, conn.DstPort, "TCP", 0)
		if err := export.WritePCAPPacket(f, conn.LastUpdate, frame); err != nil {
			continue
		}
		packetCount++
	}

	// Write JSONL sidecar file (rustnet-compatible enrichment data)
	jsonlPath := exportPath + ".jsonl"
	jsonlFile, _ := os.Create(jsonlPath)
	if jsonlFile != nil {
		for _, flow := range flows {
			fmt.Fprintf(jsonlFile, `{"srcIp":"%s","dstIp":"%s","dstPort":%d,"dstDomain":"%s","ipScope":"%s","comm":"%s","bytesOut":%d,"riskScore":%.2f}`+"\n",
				flow.SrcIP, flow.DstIP, flow.DstPort, flow.DstDomain, flow.IPScope,
				func() string {
					if len(flow.ProcessComms) > 0 {
						return flow.ProcessComms[0]
					}
					return ""
				}(),
				flow.BytesOut, flow.RiskScore)
		}
		jsonlFile.Close()
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       "exported",
		"path":         exportPath,
		"jsonlPath":    jsonlPath,
		"packetCount":  packetCount,
		"flowCount":    len(flows),
		"tcpConnCount": len(tcpConns),
	})
}
