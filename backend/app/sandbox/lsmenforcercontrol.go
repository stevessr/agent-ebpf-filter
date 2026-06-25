package sandbox

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section lsmenforcercontrol.go ----

// ── BPF LSM policy map operations ─────────────────────────────────────

func lsmPathKeyFromString(path string) (lsmPathKey, error) {
	var key lsmPathKey
	normalized, err := normalizeLsmPathString(path)
	if err != nil {
		return key, err
	}
	copy(key.Path[:], normalized)
	return key, nil
}

func normalizeLsmPathString(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("empty exec path")
	}
	if len(trimmed) >= 256 {
		return "", fmt.Errorf("exec path too long: max %d bytes", 255)
	}
	return trimmed, nil
}

func lsmNameKeyFromString(name string) (lsmNameKey, error) {
	return lsmNameKeyFromStringWithLabel(name, "file name")
}

func lsmExecNameKeyFromString(name string) (lsmNameKey, error) {
	return lsmNameKeyFromStringWithLabel(name, "exec name")
}

func lsmNameKeyFromStringWithLabel(name, label string) (lsmNameKey, error) {
	var key lsmNameKey
	normalized, err := normalizeLsmNameStringWithLabel(name, label)
	if err != nil {
		return key, err
	}
	copy(key.Name[:], normalized)
	return key, nil
}

func normalizeLsmNameStringWithLabel(name, label string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("empty %s", label)
	}
	trimmed = filepath.Base(trimmed)
	if trimmed == "." || trimmed == string(os.PathSeparator) {
		return "", fmt.Errorf("invalid %s", label)
	}
	if len(trimmed) >= 64 {
		return "", fmt.Errorf("%s too long: max %d bytes", label, 63)
	}
	return trimmed, nil
}

func blockLsmExecPath(path string) error {
	key, err := lsmPathKeyFromString(path)
	if err != nil {
		return err
	}
	snap := currentLsmEnforcerSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			return err
		}
		snap = currentLsmEnforcerSnapshot()
	}
	if snap.ExecPathBlocklist == nil {
		return fmt.Errorf("BPF LSM enforcer not loaded")
	}
	val := uint32(1)
	return snap.ExecPathBlocklist.Put(&key, &val)
}

func unblockLsmExecPath(path string) error {
	key, err := lsmPathKeyFromString(path)
	if err != nil {
		return err
	}
	snap := currentLsmEnforcerSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			return err
		}
		snap = currentLsmEnforcerSnapshot()
	}
	if snap.ExecPathBlocklist == nil {
		return fmt.Errorf("BPF LSM enforcer not loaded")
	}
	return ignoreMissingMapKey(snap.ExecPathBlocklist.Delete(&key))
}

func blockLsmExecName(name string) error {
	key, err := lsmExecNameKeyFromString(name)
	if err != nil {
		return err
	}
	snap := currentLsmEnforcerSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			return err
		}
		snap = currentLsmEnforcerSnapshot()
	}
	if snap.ExecNameBlocklist == nil {
		return fmt.Errorf("BPF LSM exec-name blocklist not loaded")
	}
	val := uint32(1)
	return snap.ExecNameBlocklist.Put(&key, &val)
}

func unblockLsmExecName(name string) error {
	key, err := lsmExecNameKeyFromString(name)
	if err != nil {
		return err
	}
	snap := currentLsmEnforcerSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			return err
		}
		snap = currentLsmEnforcerSnapshot()
	}
	if snap.ExecNameBlocklist == nil {
		return fmt.Errorf("BPF LSM exec-name blocklist not loaded")
	}
	return ignoreMissingMapKey(snap.ExecNameBlocklist.Delete(&key))
}

func blockLsmFileName(name string) error {
	key, err := lsmNameKeyFromString(name)
	if err != nil {
		return err
	}
	snap := currentLsmEnforcerSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			return err
		}
		snap = currentLsmEnforcerSnapshot()
	}
	if snap.FileNameBlocklist == nil {
		return fmt.Errorf("BPF LSM enforcer not loaded")
	}
	val := uint32(1)
	return snap.FileNameBlocklist.Put(&key, &val)
}

func unblockLsmFileName(name string) error {
	key, err := lsmNameKeyFromString(name)
	if err != nil {
		return err
	}
	snap := currentLsmEnforcerSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			return err
		}
		snap = currentLsmEnforcerSnapshot()
	}
	if snap.FileNameBlocklist == nil {
		return fmt.Errorf("BPF LSM enforcer not loaded")
	}
	return ignoreMissingMapKey(snap.FileNameBlocklist.Delete(&key))
}

func getLsmEnforcerStats(statsMap *ebpf.Map) (lsmEnforcerStats, error) {
	if statsMap == nil {
		return lsmEnforcerStats{}, fmt.Errorf("BPF LSM stats map not loaded")
	}

	cpuCount, err := ebpf.PossibleCPU()
	if err != nil || cpuCount <= 0 {
		return lsmEnforcerStats{}, err
	}

	type rawStats struct {
		ExecChecked uint64
		ExecBlocked uint64
		FileChecked uint64
		FileBlocked uint64
	}

	values := make([]rawStats, cpuCount)
	key := uint32(0)
	if err := statsMap.Lookup(&key, &values); err != nil {
		return lsmEnforcerStats{}, err
	}

	var total lsmEnforcerStats
	for _, s := range values {
		total.ExecChecked += s.ExecChecked
		total.ExecBlocked += s.ExecBlocked
		total.FileChecked += s.FileChecked
		total.FileBlocked += s.FileBlocked
	}
	return total, nil
}

func listLsmExecPaths(blocklist *ebpf.Map) []string {
	if blocklist == nil {
		return nil
	}
	items := []string{}
	iter := blocklist.Iterate()
	var key lsmPathKey
	var val uint32
	for iter.Next(&key, &val) {
		if val == 0 {
			continue
		}
		items = append(items, string(bytes.TrimRight(key.Path[:], "\x00")))
	}
	return items
}

func listLsmExecNames(blocklist *ebpf.Map) []string {
	if blocklist == nil {
		return nil
	}
	items := []string{}
	iter := blocklist.Iterate()
	var key lsmNameKey
	var val uint32
	for iter.Next(&key, &val) {
		if val == 0 {
			continue
		}
		items = append(items, string(bytes.TrimRight(key.Name[:], "\x00")))
	}
	return items
}

func listLsmFileNames(blocklist *ebpf.Map) []string {
	if blocklist == nil {
		return nil
	}
	items := []string{}
	iter := blocklist.Iterate()
	var key lsmNameKey
	var val uint32
	for iter.Next(&key, &val) {
		if val == 0 {
			continue
		}
		items = append(items, string(bytes.TrimRight(key.Name[:], "\x00")))
	}
	return items
}

// ── HTTP handlers ─────────────────────────────────────────────────────

func handleLsmEnforcerStatus(c *gin.Context) {
	snap := currentLsmEnforcerSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			log.Printf("[LSM-ENFORCER] status-triggered load failed: %v", err)
		}
		snap = currentLsmEnforcerSnapshot()
	}
	stats, err := getLsmEnforcerStats(snap.Stats)
	statsError := ""
	if err != nil {
		statsError = err.Error()
	}
	available := snap.available()
	attached := snap.attached()
	c.JSON(http.StatusOK, gin.H{
		"available": available,
		"attached":  attached,
		"linkCount": snap.LinkCount,
		"linkPins":  snap.LinkPins,
		"maps": gin.H{
			"execPathBlocklist": snap.ExecPathBlocklist != nil,
			"execNameBlocklist": snap.ExecNameBlocklist != nil,
			"fileNameBlocklist": snap.FileNameBlocklist != nil,
			"stats":             snap.Stats != nil,
		},
		"blockedExecPaths": listLsmExecPaths(snap.ExecPathBlocklist),
		"blockedExecNames": listLsmExecNames(snap.ExecNameBlocklist),
		"blockedFileNames": listLsmFileNames(snap.FileNameBlocklist),
		"stats":            stats,
		"statsError":       statsError,
		"error":            snap.LastError,
	})
}

func handleLsmBlockExecPath(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	path, err := normalizeLsmPathString(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := blockLsmExecPath(path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "path": path})
}

func handleLsmUnblockExecPath(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	path, err := normalizeLsmPathString(req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := unblockLsmExecPath(path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "path": path})
}

func handleLsmBlockExecName(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, err := normalizeLsmNameStringWithLabel(req.Name, "exec name")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := blockLsmExecName(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "name": name})
}

func handleLsmUnblockExecName(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, err := normalizeLsmNameStringWithLabel(req.Name, "exec name")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := unblockLsmExecName(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "name": name})
}

func handleLsmBlockFileName(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, err := normalizeLsmNameStringWithLabel(req.Name, "file name")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := blockLsmFileName(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "blocked", "name": name})
}

func handleLsmUnblockFileName(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, err := normalizeLsmNameStringWithLabel(req.Name, "file name")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := unblockLsmFileName(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "unblocked", "name": name})
}
