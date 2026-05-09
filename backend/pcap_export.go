package main

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

// ── Minimal PCAP writer (libpcap format) ──────────────────────────────

const (
	pcapMagicNumber   = 0xa1b2c3d4
	pcapVersionMajor  = 2
	pcapVersionMinor  = 4
	pcapThisZone      = 0
	pcapSigFigs       = 0
	pcapSnapLen       = 65535
	pcapLinkTypeEthernet = 1
)

type pcapGlobalHeader struct {
	MagicNumber  uint32
	VersionMajor uint16
	VersionMinor uint16
	ThisZone     int32
	SigFigs      uint32
	SnapLen      uint32
	LinkType     uint32
}

type pcapPacketHeader struct {
	TSSeconds   uint32
	TSUseconds  uint32
	IncludedLen uint32
	OrigLen     uint32
}

func writePCAPHeader(f *os.File) error {
	hdr := pcapGlobalHeader{
		MagicNumber:  pcapMagicNumber,
		VersionMajor: pcapVersionMajor,
		VersionMinor: pcapVersionMinor,
		SnapLen:      pcapSnapLen,
		LinkType:     pcapLinkTypeEthernet,
	}
	return binary.Write(f, binary.LittleEndian, hdr)
}

func writePCAPPacket(f *os.File, timestamp time.Time, data []byte) error {
	ts := timestamp.UTC()
	pktHdr := pcapPacketHeader{
		TSSeconds:   uint32(ts.Unix()),
		TSUseconds:  uint32(ts.Nanosecond() / 1000),
		IncludedLen: uint32(len(data)),
		OrigLen:     uint32(len(data)),
	}
	if err := binary.Write(f, binary.LittleEndian, pktHdr); err != nil {
		return err
	}
	_, err := f.Write(data)
	return err
}

// Build a synthetic Ethernet frame for a network flow.
// This is a minimal frame for Wireshark display — not a real captured packet.
func buildSyntheticEthernetFrame(srcIP, dstIP string, srcPort, dstPort uint32, protocol string, bytes uint64) []byte {
	frame := make([]byte, 14+20+20) // Eth(14) + IP(20) + TCP(20) = 54 bytes

	// Ethernet header
	frame[12] = 0x08 // EtherType: IPv4
	frame[13] = 0x00

	// IPv4 header
	frame[14] = 0x45                 // Version=4, IHL=5
	frame[15] = 0x00                 // DSCP/ECN
	binary.BigEndian.PutUint16(frame[16:18], uint16(len(frame)-14)) // Total length
	frame[18] = 0x00                 // Identification
	frame[19] = 0x01
	frame[20] = 0x00 // Flags + Fragment
	frame[21] = 0x00
	frame[22] = 0x40 // TTL=64
	frame[23] = 0x06 // Protocol=TCP
	// Checksum at 24-25 (leave 0)
	frame[29] = 0x06 // Protocol=TCP (byte 9 of IP header)

	// Source IP (bytes 26-29)
	parseIPToBytes(srcIP, frame[26:30])
	// Dest IP (bytes 30-33)
	parseIPToBytes(dstIP, frame[30:34])

	// TCP header (starts at offset 34)
	binary.BigEndian.PutUint16(frame[34:36], uint16(srcPort))
	binary.BigEndian.PutUint16(frame[36:38], uint16(dstPort))
	// Sequence number
	frame[38] = 0x00
	frame[39] = 0x00
	frame[40] = 0x00
	frame[41] = 0x01
	// Ack number
	frame[46] = 0x50 // Data offset=5 (20 bytes), flags
	frame[47] = 0x02 // SYN flag
	// Window size
	frame[48] = 0xFF
	frame[49] = 0xFF

	return frame
}

func parseIPToBytes(ipStr string, dst []byte) {
	// Simple dotted-decimal parser
	var a, b, c, d int
	fmt.Sscanf(ipStr, "%d.%d.%d.%d", &a, &b, &c, &d)
	dst[0] = byte(a)
	dst[1] = byte(b)
	dst[2] = byte(c)
	dst[3] = byte(d)
}

// ── PCAP export handler ──────────────────────────────────────────────

func handlePCAPExport(c *gin.Context) {
	flows := networkFlowAggregator.Snapshot()
	tcpConns := tcpTracker.Snapshot()

	exportDir := runtimeSettingsDir()
	exportPath := filepath.Join(exportDir, fmt.Sprintf("network-export-%s.pcap", time.Now().UTC().Format("20060102-150405")))

	f, err := os.Create(exportPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer f.Close()

	if err := writePCAPHeader(f); err != nil {
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

		frame := buildSyntheticEthernetFrame(srcIP, dstIP, flow.SrcPort, flow.DstPort, flow.Protocol, flow.BytesOut)
		if err := writePCAPPacket(f, time.UnixMilli(flow.FirstSeen), frame); err != nil {
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

		frame := buildSyntheticEthernetFrame(srcIP, dstIP, conn.SrcPort, conn.DstPort, "TCP", 0)
		if err := writePCAPPacket(f, conn.LastUpdate, frame); err != nil {
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
				func() string { if len(flow.ProcessComms) > 0 { return flow.ProcessComms[0] }; return "" }(),
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
