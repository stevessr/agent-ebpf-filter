package app

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section cgroupsandboxhandlers.go ----

// ── HTTP handlers ─────────────────────────────────────────────────────

func handleCgroupSandboxStatus(c *gin.Context) {
	snap := currentCgroupSandboxSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureCgroupSandboxLoaded(); err != nil {
			log.Printf("[CGROUP-SANDBOX] status-triggered load failed: %v", err)
		}
		snap = currentCgroupSandboxSnapshot()
	}
	stats, err := getCgroupSandboxStats(snap.SandboxStats)
	available := snap.available()
	statsError := ""
	if err != nil {
		statsError = err.Error()
	}
	attached := snap.attached()

	c.JSON(http.StatusOK, gin.H{
		"available":      available,
		"attached":       attached,
		"cgroupPath":     snap.CgroupPath,
		"linkCount":      snap.LinkCount,
		"linkPins":       snap.LinkPins,
		"blockedCgroups": listBlockedCgroups(snap.CgroupBlocklist),
		"blockedIPs":     listBlockedIPs(snap.IPBlocklist, snap.IP6Blocklist),
		"blockedPorts":   listBlockedPorts(snap.PortBlocklist),
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

func handleCgroupSandboxBlockCgroup(c *gin.Context) {
	var req cgroupIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cgroupID, err := parseCgroupID(req.CgroupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := blockCgroup(cgroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "cgroupId": fmt.Sprintf("%d", cgroupID)})
}

func handleCgroupSandboxUnblockCgroup(c *gin.Context) {
	var req cgroupIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cgroupID, err := parseCgroupID(req.CgroupID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := unblockCgroup(cgroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "cgroupId": fmt.Sprintf("%d", cgroupID)})
}

func handleCgroupSandboxBlockPID(c *gin.Context) {
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
	snap := currentCgroupSandboxSnapshot()
	cgroupID, cgroupPath, err := cgroupIDForPID(req.PID, snap.CgroupPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := blockCgroup(cgroupID); err != nil {
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

func handleCgroupSandboxUnblockPID(c *gin.Context) {
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
	snap := currentCgroupSandboxSnapshot()
	cgroupID, cgroupPath, err := cgroupIDForPID(req.PID, snap.CgroupPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := unblockCgroup(cgroupID); err != nil {
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

func handleCgroupSandboxBlockIP(c *gin.Context) {
	var req struct {
		IP string `json:"ip"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, ipText, err := parseCgroupSandboxIP(req.IP)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := blockIP(ipText); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "ip": ipText})
}

func handleCgroupSandboxUnblockIP(c *gin.Context) {
	var req struct {
		IP string `json:"ip"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, ipText, err := parseCgroupSandboxIP(req.IP)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := unblockIP(ipText); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "ip": ipText})
}

func handleCgroupSandboxBlockPort(c *gin.Context) {
	var req struct {
		Port uint16 `json:"port"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateCgroupSandboxPort(req.Port); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := blockPort(req.Port); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "port": req.Port})
}

func handleCgroupSandboxUnblockPort(c *gin.Context) {
	var req struct {
		Port uint16 `json:"port"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateCgroupSandboxPort(req.Port); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := unblockPort(req.Port); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "port": req.Port})
}
