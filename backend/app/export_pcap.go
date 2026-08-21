package app

import (
	"agent-ebpf-filter/app/export"
	"agent-ebpf-filter/app/platform"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// handlePCAPExport is the HTTP handler for PCAP export.
// The PCAP format writing primitives live in the export subpackage.

func handlePCAPExport(c *gin.Context) {
	flows := currentNetworkFlowAggregator().Snapshot()
	tcpConns := currentTCPConnections()

	exportDir := platform.RuntimeSettingsDir()
	exportPath, jsonlPath, packetCount, err := writePCAPExportFiles(exportDir, flows, tcpConns)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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

func writePCAPExportFiles(exportDir string, flows []NetworkFlowSummary, tcpConns []tcpConnectionState) (exportPath, jsonlPath string, packetCount int, err error) {
	if err := os.MkdirAll(exportDir, 0o700); err != nil {
		return "", "", 0, fmt.Errorf("create PCAP export directory: %w", err)
	}
	f, err := os.CreateTemp(exportDir, "network-export-*.pcap")
	if err != nil {
		return "", "", 0, fmt.Errorf("create PCAP export: %w", err)
	}
	exportPath = f.Name()
	removeFiles := true
	defer func() {
		_ = f.Close()
		if removeFiles {
			_ = os.Remove(exportPath)
			if jsonlPath != "" {
				_ = os.Remove(jsonlPath)
			}
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return "", "", 0, fmt.Errorf("set PCAP permissions: %w", err)
	}
	if err := platform.ChownArtifactFile(f); err != nil {
		return "", "", 0, fmt.Errorf("set PCAP ownership: %w", err)
	}
	if err := export.WritePCAPHeader(f); err != nil {
		return "", "", 0, fmt.Errorf("write PCAP header: %w", err)
	}

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
	if err := f.Sync(); err != nil {
		return "", "", 0, fmt.Errorf("sync PCAP export: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", "", 0, fmt.Errorf("close PCAP export: %w", err)
	}

	// Write JSONL sidecar file (rustnet-compatible enrichment data)
	jsonlPath = exportPath + ".jsonl"
	jsonlFile, err := os.OpenFile(jsonlPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", "", 0, fmt.Errorf("create PCAP sidecar: %w", err)
	}
	if err := platform.ChownArtifactFile(jsonlFile); err != nil {
		_ = jsonlFile.Close()
		return "", "", 0, fmt.Errorf("set PCAP sidecar ownership: %w", err)
	}
	encoder := json.NewEncoder(jsonlFile)
	for _, flow := range flows {
		comm := ""
		if len(flow.ProcessComms) > 0 {
			comm = flow.ProcessComms[0]
		}
		if err := encoder.Encode(map[string]any{
			"srcIp": flow.SrcIP, "dstIp": flow.DstIP, "dstPort": flow.DstPort,
			"dstDomain": flow.DstDomain, "ipScope": flow.IPScope, "comm": comm,
			"bytesOut": flow.BytesOut, "riskScore": flow.RiskScore,
		}); err != nil {
			_ = jsonlFile.Close()
			return "", "", 0, fmt.Errorf("write PCAP sidecar: %w", err)
		}
	}
	if err := syncAndCloseFile(jsonlFile); err != nil {
		return "", "", 0, fmt.Errorf("finalize PCAP sidecar: %w", err)
	}
	removeFiles = false
	return exportPath, jsonlPath, packetCount, nil
}

func syncAndCloseFile(file *os.File) error {
	if file == nil {
		return nil
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
