package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ---- moved from app/lsmenforcercontrol.go ----

// HandleLsmEnforcerStatus returns the current LSM enforcer state.
func HandleLsmEnforcerStatus(c *gin.Context) {
	snap := Deps.LsmEnforcer.Snapshot()
	if !snap.Available || !snap.Attached {
		if err := Deps.LsmEnforcer.EnsureLoaded(); err != nil {
			log.Printf("[LSM-ENFORCER] status-triggered load failed: %v", err)
		}
		snap = Deps.LsmEnforcer.Snapshot()
	}
	stats, err := Deps.LsmEnforcer.GetStats(snap.Stats)
	statsError := ""
	if err != nil {
		statsError = err.Error()
	}
	c.JSON(http.StatusOK, gin.H{
		"available": snap.Available,
		"attached":  snap.Attached,
		"linkCount": snap.LinkCount,
		"linkPins":  snap.LinkPins,
		"maps": gin.H{
			"execPathBlocklist": snap.ExecPathBlocklist != nil,
			"execNameBlocklist": snap.ExecNameBlocklist != nil,
			"fileNameBlocklist": snap.FileNameBlocklist != nil,
			"stats":             snap.Stats != nil,
		},
		"blockedExecPaths": Deps.LsmEnforcer.ListExecPaths(snap.ExecPathBlocklist),
		"blockedExecNames": Deps.LsmEnforcer.ListExecNames(snap.ExecNameBlocklist),
		"blockedFileNames": Deps.LsmEnforcer.ListFileNames(snap.FileNameBlocklist),
		"stats":            stats,
		"statsError":       statsError,
		"error":            snap.LastError,
	})
}

// HandleLsmBlockExecPath blocks an executable path via BPF LSM.
func HandleLsmBlockExecPath(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	path, err := Deps.LsmEnforcer.NormalizePath(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.LsmEnforcer.BlockExecPath(path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "path": path})
}

// HandleLsmUnblockExecPath removes an executable-path block.
func HandleLsmUnblockExecPath(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	path, err := Deps.LsmEnforcer.NormalizePath(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.LsmEnforcer.UnblockExecPath(path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "path": path})
}

// HandleLsmBlockExecName blocks an executable basename via BPF LSM.
func HandleLsmBlockExecName(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, err := Deps.LsmEnforcer.NormalizeName(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.LsmEnforcer.BlockExecName(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "name": name})
}

// HandleLsmUnblockExecName removes an executable-basename block.
func HandleLsmUnblockExecName(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, err := Deps.LsmEnforcer.NormalizeName(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.LsmEnforcer.UnblockExecName(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "name": name})
}

// HandleLsmBlockFileName blocks a file/directory basename via BPF LSM.
func HandleLsmBlockFileName(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, err := Deps.LsmEnforcer.NormalizeName(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.LsmEnforcer.BlockFileName(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "name": name})
}

// HandleLsmUnblockFileName removes a file/directory basename block.
func HandleLsmUnblockFileName(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, err := Deps.LsmEnforcer.NormalizeName(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.LsmEnforcer.UnblockFileName(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "name": name})
}
