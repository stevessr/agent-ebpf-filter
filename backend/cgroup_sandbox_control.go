package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"
)

// ── Cgroup sandbox eBPF map management ────────────────────────────────

type cgroupSandboxMaps struct {
	CgroupBlocklist    *ebpf.Map
	IPBlocklist        *ebpf.Map
	PortBlocklist      *ebpf.Map
	SandboxStats       *ebpf.Map
}

var cgroupSandbox cgroupSandboxMaps

const cgroupSandboxPinRoot = ebpfPinRoot + "/cgroup_sandbox"
const cgroupSandboxMapsDir = cgroupSandboxPinRoot + "/maps"

func ensureCgroupSandboxLoaded() error {
	// Try to load pinned maps
	cgroupBlocklist, err := ebpf.LoadPinnedMap(cgroupSandboxMapsDir+"/cgroup_blocklist", nil)
	if err != nil {
		return fmt.Errorf("cgroup sandbox maps not available: %w (kernel may not support cgroup/connect4)", err)
	}
	cgroupSandbox.CgroupBlocklist = cgroupBlocklist

	ipBlocklist, _ := ebpf.LoadPinnedMap(cgroupSandboxMapsDir+"/ip_blocklist", nil)
	if ipBlocklist != nil {
		cgroupSandbox.IPBlocklist = ipBlocklist
	}

	portBlocklist, _ := ebpf.LoadPinnedMap(cgroupSandboxMapsDir+"/port_blocklist", nil)
	if portBlocklist != nil {
		cgroupSandbox.PortBlocklist = portBlocklist
	}

	stats, _ := ebpf.LoadPinnedMap(cgroupSandboxMapsDir+"/cgroup_sandbox_stats", nil)
	if stats != nil {
		cgroupSandbox.SandboxStats = stats
	}

	log.Printf("[CGROUP-SANDBOX] loaded: cgroup=%v ip=%v port=%v stats=%v",
		cgroupSandbox.CgroupBlocklist != nil,
		cgroupSandbox.IPBlocklist != nil,
		cgroupSandbox.PortBlocklist != nil,
		cgroupSandbox.SandboxStats != nil)

	return nil
}

// ── Management operations ─────────────────────────────────────────────

func blockCgroup(cgroupID uint64) error {
	if cgroupSandbox.CgroupBlocklist == nil {
		return fmt.Errorf("cgroup sandbox not loaded")
	}
	val := uint32(1)
	return cgroupSandbox.CgroupBlocklist.Put(&cgroupID, &val)
}

func unblockCgroup(cgroupID uint64) error {
	if cgroupSandbox.CgroupBlocklist == nil {
		return fmt.Errorf("cgroup sandbox not loaded")
	}
	return cgroupSandbox.CgroupBlocklist.Delete(&cgroupID)
}

func blockIP(ipStr string) error {
	if cgroupSandbox.IPBlocklist == nil {
		return fmt.Errorf("cgroup sandbox IP blocklist not loaded")
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return fmt.Errorf("invalid IP: %s", ipStr)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("only IPv4 supported: %s", ipStr)
	}
	ipU32 := binary.BigEndian.Uint32(ip4) // network byte order
	val := uint32(1)
	return cgroupSandbox.IPBlocklist.Put(&ipU32, &val)
}

func unblockIP(ipStr string) error {
	if cgroupSandbox.IPBlocklist == nil {
		return fmt.Errorf("cgroup sandbox IP blocklist not loaded")
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return nil
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return nil
	}
	ipU32 := binary.BigEndian.Uint32(ip4)
	return cgroupSandbox.IPBlocklist.Delete(&ipU32)
}

func blockPort(port uint16) error {
	if cgroupSandbox.PortBlocklist == nil {
		return fmt.Errorf("cgroup sandbox port blocklist not loaded")
	}
	portU32 := uint32(port)
	val := uint32(1)
	return cgroupSandbox.PortBlocklist.Put(&portU32, &val)
}

func unblockPort(port uint16) error {
	if cgroupSandbox.PortBlocklist == nil {
		return fmt.Errorf("cgroup sandbox port blocklist not loaded")
	}
	portU32 := uint32(port)
	return cgroupSandbox.PortBlocklist.Delete(&portU32)
}

// ── Statistics ────────────────────────────────────────────────────────

type cgroupSandboxStats struct {
	ConnectChecked uint64 `json:"connectChecked"`
	ConnectBlocked uint64 `json:"connectBlocked"`
	ConnectAllowed uint64 `json:"connectAllowed"`
}

func getCgroupSandboxStats() (cgroupSandboxStats, error) {
	if cgroupSandbox.SandboxStats == nil {
		return cgroupSandboxStats{}, fmt.Errorf("stats map not loaded")
	}

	cpuCount, err := ebpf.PossibleCPU()
	if err != nil || cpuCount <= 0 {
		return cgroupSandboxStats{}, err
	}

	type rawStats struct {
		ConnectChecked uint64
		ConnectBlocked uint64
		ConnectAllowed uint64
	}

	values := make([]rawStats, cpuCount)
	key := uint32(0)
	if err := cgroupSandbox.SandboxStats.Lookup(&key, &values); err != nil {
		return cgroupSandboxStats{}, err
	}

	var total cgroupSandboxStats
	for _, s := range values {
		total.ConnectChecked += s.ConnectChecked
		total.ConnectBlocked += s.ConnectBlocked
		total.ConnectAllowed += s.ConnectAllowed
	}
	return total, nil
}

// ── HTTP handlers ─────────────────────────────────────────────────────

func handleCgroupSandboxStatus(c *gin.Context) {
	stats, err := getCgroupSandboxStats()
	available := err == nil

	c.JSON(http.StatusOK, gin.H{
		"available": available,
		"stats":     stats,
		"error":     func() string { if err != nil { return err.Error() }; return "" }(),
	})
}

func handleCgroupSandboxBlockCgroup(c *gin.Context) {
	var req struct {
		CgroupID uint64 `json:"cgroupId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := blockCgroup(req.CgroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "cgroupId": req.CgroupID})
}

func handleCgroupSandboxUnblockCgroup(c *gin.Context) {
	var req struct {
		CgroupID uint64 `json:"cgroupId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := unblockCgroup(req.CgroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "cgroupId": req.CgroupID})
}

func handleCgroupSandboxBlockIP(c *gin.Context) {
	var req struct {
		IP string `json:"ip"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := blockIP(req.IP); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "ip": req.IP})
}

func handleCgroupSandboxBlockPort(c *gin.Context) {
	var req struct {
		Port uint16 `json:"port"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := blockPort(req.Port); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "port": req.Port})
}

// ── Auto-block high-risk endpoints ────────────────────────────────────

func autoBlockHighRiskEndpoints() {
	// Block known C2 ports
	highRiskPorts := []uint16{4444, 1337, 31337, 6666, 6667, 9999}
	for _, port := range highRiskPorts {
		if err := blockPort(port); err != nil {
			log.Printf("[CGROUP-SANDBOX] auto-block port %d: %v", port, err)
		}
	}
	log.Printf("[CGROUP-SANDBOX] auto-blocked %d high-risk ports", len(highRiskPorts))
}

// Apply cgroup sandbox to a specific agent run (block all outbound for that cgroup)
func sandboxCgroupForAgent(cgroupID uint64) error {
	if cgroupSandbox.CgroupBlocklist == nil {
		return fmt.Errorf("cgroup sandbox not available")
	}
	return blockCgroup(cgroupID)
}

// Release cgroup sandbox (allow outbound again)
func releaseCgroupSandbox(cgroupID uint64) error {
	if cgroupSandbox.CgroupBlocklist == nil {
		return nil
	}
	return unblockCgroup(cgroupID)
}
