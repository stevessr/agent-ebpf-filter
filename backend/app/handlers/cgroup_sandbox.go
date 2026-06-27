package handlers

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ---- moved from app/cgroupsandboxhandlers.go ----

// HandleCgroupSandboxStatus returns the current cgroup sandbox state.
func HandleCgroupSandboxStatus(c *gin.Context) {
	snap := Deps.CgroupSandbox.Snapshot()
	if !snap.Available || !snap.Attached {
		if err := Deps.CgroupSandbox.EnsureLoaded(); err != nil {
			log.Printf("[CGROUP-SANDBOX] status-triggered load failed: %v", err)
		}
		snap = Deps.CgroupSandbox.Snapshot()
	}
	stats, err := Deps.CgroupSandbox.GetStats(snap.SandboxStats)
	statsError := ""
	if err != nil {
		statsError = err.Error()
	}

	c.JSON(http.StatusOK, gin.H{
		"available":      snap.Available,
		"attached":       snap.Attached,
		"cgroupPath":     snap.CgroupPath,
		"linkCount":      snap.LinkCount,
		"linkPins":       snap.LinkPins,
		"blockedCgroups": Deps.CgroupSandbox.ListBlockedCgroups(snap.CgroupBlocklist),
		"blockedIPs":     Deps.CgroupSandbox.ListBlockedIPs(snap.IPBlocklist, snap.IP6Blocklist),
		"blockedPorts":   Deps.CgroupSandbox.ListBlockedPorts(snap.PortBlocklist),
		"maps": gin.H{
			"cgroupBlocklist": snap.CgroupBlocklist != nil,
			"ipBlocklist":     snap.IPBlocklist != nil,
			"ip6Blocklist":    snap.IP6Blocklist != nil,
			"portBlocklist":   snap.PortBlocklist != nil,
			"stats":           snap.SandboxStats != nil,
		},
		"stats":      stats,
		"statsError": statsError,
		"error":      snap.LastError,
	})
}

// HandleCgroupSandboxBlockCgroup blocks a cgroup by ID.
func HandleCgroupSandboxBlockCgroup(c *gin.Context) {
	var req struct {
		CgroupID string `json:"cgroup_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cgroupID, err := Deps.CgroupSandbox.ParseCgroupID(req.CgroupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.CgroupSandbox.BlockCgroup(cgroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "cgroupId": fmt.Sprintf("%d", cgroupID)})
}

// HandleCgroupSandboxUnblockCgroup unblocks a cgroup by ID.
func HandleCgroupSandboxUnblockCgroup(c *gin.Context) {
	var req struct {
		CgroupID string `json:"cgroup_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cgroupID, err := Deps.CgroupSandbox.ParseCgroupID(req.CgroupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.CgroupSandbox.UnblockCgroup(cgroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "cgroupId": fmt.Sprintf("%d", cgroupID)})
}

// HandleCgroupSandboxBlockPID blocks networking for a PID.
func HandleCgroupSandboxBlockPID(c *gin.Context) {
	var req struct {
		PID int `json:"pid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.PID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid pid: %d", req.PID)})
		return
	}
	snap := Deps.CgroupSandbox.Snapshot()
	cgroupID, cgroupPath, err := Deps.CgroupSandbox.CgroupIDForPID(req.PID, snap.CgroupPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.CgroupSandbox.BlockCgroup(cgroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":     "blocked",
		"pid":        req.PID,
		"cgroupId":   fmt.Sprintf("%d", cgroupID),
		"cgroupPath": cgroupPath,
	})
}

// HandleCgroupSandboxUnblockPID unblocks networking for a PID.
func HandleCgroupSandboxUnblockPID(c *gin.Context) {
	var req struct {
		PID int `json:"pid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.PID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid pid: %d", req.PID)})
		return
	}
	snap := Deps.CgroupSandbox.Snapshot()
	cgroupID, cgroupPath, err := Deps.CgroupSandbox.CgroupIDForPID(req.PID, snap.CgroupPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.CgroupSandbox.UnblockCgroup(cgroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":     "unblocked",
		"pid":        req.PID,
		"cgroupId":   fmt.Sprintf("%d", cgroupID),
		"cgroupPath": cgroupPath,
	})
}

// HandleCgroupSandboxBlockIP blocks a destination IP.
func HandleCgroupSandboxBlockIP(c *gin.Context) {
	var req struct {
		IP string `json:"ip"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, ipText, err := Deps.CgroupSandbox.ParseIP(req.IP)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.CgroupSandbox.BlockIP(ipText); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "ip": ipText})
}

// HandleCgroupSandboxUnblockIP removes a destination IP block.
func HandleCgroupSandboxUnblockIP(c *gin.Context) {
	var req struct {
		IP string `json:"ip"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, ipText, err := Deps.CgroupSandbox.ParseIP(req.IP)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.CgroupSandbox.UnblockIP(ipText); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "ip": ipText})
}

// HandleCgroupSandboxBlockPort blocks a TCP/UDP destination port.
func HandleCgroupSandboxBlockPort(c *gin.Context) {
	var req struct {
		Port uint16 `json:"port"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.CgroupSandbox.ValidatePort(req.Port); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.CgroupSandbox.BlockPort(req.Port); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "port": req.Port})
}

// HandleCgroupSandboxUnblockPort removes a TCP/UDP destination-port block.
func HandleCgroupSandboxUnblockPort(c *gin.Context) {
	var req struct {
		Port uint16 `json:"port"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.CgroupSandbox.ValidatePort(req.Port); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.CgroupSandbox.UnblockPort(req.Port); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "port": req.Port})
}
