package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/gin-gonic/gin"
)

// ── BPF LSM enforcement map and link management ────────────────────────

const lsmEnforcerPinRoot = ebpfPinRoot + "/lsm_enforcer"
const lsmEnforcerMapsDir = lsmEnforcerPinRoot + "/maps"
const lsmEnforcerLinksDir = lsmEnforcerPinRoot + "/links"
const lsmEnforcerMapPinMode os.FileMode = 0600
const expectedLsmEnforcerLinks = 14

type lsmEnforcerRuntime struct {
	ExecPathBlocklist *ebpf.Map
	ExecNameBlocklist *ebpf.Map
	FileNameBlocklist *ebpf.Map
	Stats             *ebpf.Map
	Links             []link.Link
	LinkPins          []string
	LastError         string
}

type lsmPathKey struct {
	Path [256]byte
}

type lsmNameKey struct {
	Name [64]byte
}

type lsmEnforcerStats struct {
	ExecChecked uint64 `json:"execChecked"`
	ExecBlocked uint64 `json:"execBlocked"`
	FileChecked uint64 `json:"fileChecked"`
	FileBlocked uint64 `json:"fileBlocked"`
}

var lsmEnforcer lsmEnforcerRuntime
var lsmEnforcerMu sync.RWMutex
var errLsmEnforcerPinnedLinksMissing = errors.New("BPF LSM pinned links missing")

func ensureLsmEnforcerLoaded() error {
	lsmEnforcerMu.Lock()
	defer lsmEnforcerMu.Unlock()

	if lsmEnforcerAvailableLocked() && lsmEnforcerAttachedLocked() {
		return nil
	}

	if pinnedMaps, err := loadPinnedLsmEnforcerMaps(); err == nil {
		closeMapHandles(pinnedMaps)
		if err := attachLsmEnforcerWithPinnedMaps(); err != nil {
			// Preserve pinned LSM policy maps if a restarted backend cannot
			// reattach. Losing explicit kernel-deny policy is worse than asking
			// the operator to reset stale pins deliberately.
			lsmEnforcer.LastError = err.Error()
			return err
		}
		return nil
	}

	if err := bootstrapLsmEnforcer(); err != nil {
		lsmEnforcer.LastError = err.Error()
		return err
	}
	lsmEnforcer.LastError = ""
	return nil
}

func lsmEnforcerAvailable() bool {
	lsmEnforcerMu.RLock()
	defer lsmEnforcerMu.RUnlock()
	return lsmEnforcerAvailableLocked()
}

func lsmEnforcerAvailableLocked() bool {
	return lsmEnforcer.ExecPathBlocklist != nil &&
		lsmEnforcer.ExecNameBlocklist != nil &&
		lsmEnforcer.FileNameBlocklist != nil &&
		lsmEnforcer.Stats != nil
}

func lsmEnforcerAttached() bool {
	lsmEnforcerMu.RLock()
	defer lsmEnforcerMu.RUnlock()
	return lsmEnforcerAttachedLocked()
}

func lsmEnforcerAttachedLocked() bool {
	return len(lsmEnforcer.Links) >= expectedLsmEnforcerLinks
}

type lsmEnforcerSnapshot struct {
	ExecPathBlocklist *ebpf.Map
	ExecNameBlocklist *ebpf.Map
	FileNameBlocklist *ebpf.Map
	Stats             *ebpf.Map
	LinkCount         int
	LinkPins          []string
	LastError         string
}

func currentLsmEnforcerSnapshot() lsmEnforcerSnapshot {
	lsmEnforcerMu.RLock()
	defer lsmEnforcerMu.RUnlock()
	return lsmEnforcerSnapshot{
		ExecPathBlocklist: lsmEnforcer.ExecPathBlocklist,
		ExecNameBlocklist: lsmEnforcer.ExecNameBlocklist,
		FileNameBlocklist: lsmEnforcer.FileNameBlocklist,
		Stats:             lsmEnforcer.Stats,
		LinkCount:         len(lsmEnforcer.Links),
		LinkPins:          append([]string(nil), lsmEnforcer.LinkPins...),
		LastError:         lsmEnforcer.LastError,
	}
}

func (s lsmEnforcerSnapshot) available() bool {
	return s.ExecPathBlocklist != nil && s.ExecNameBlocklist != nil && s.FileNameBlocklist != nil && s.Stats != nil
}

func (s lsmEnforcerSnapshot) attached() bool {
	return s.LinkCount >= expectedLsmEnforcerLinks
}

func bootstrapLsmEnforcer() error {
	_ = os.RemoveAll(lsmEnforcerPinRoot)
	for _, d := range []string{lsmEnforcerMapsDir, lsmEnforcerLinksDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}

	var objs bpf.AgentLsmEnforcerObjects
	if err := bpf.LoadAgentLsmEnforcerObjects(&objs, nil); err != nil {
		return fmt.Errorf("load BPF LSM enforcer objects: %w", err)
	}
	defer objs.Close()

	if err := pinLsmEnforcerMaps(&objs); err != nil {
		return err
	}
	if err := ensureLsmEnforcerMapPermissions(); err != nil {
		return err
	}
	links, pins, err := attachLsmEnforcerPrograms(&objs)
	if err != nil {
		return err
	}
	replaceLsmEnforcerLinks(links)
	return loadLsmEnforcerRuntimeMaps(pins)
}

func attachLsmEnforcerWithPinnedMaps() error {
	replacements, err := loadPinnedLsmEnforcerMaps()
	if err != nil {
		return err
	}
	if err := ensureLsmEnforcerMapPermissions(); err != nil {
		closeMapHandles(replacements)
		return err
	}

	var objs bpf.AgentLsmEnforcerObjects
	if err := bpf.LoadAgentLsmEnforcerObjects(&objs, &ebpf.CollectionOptions{MapReplacements: replacements}); err != nil {
		closeMapHandles(replacements)
		return fmt.Errorf("load BPF LSM enforcer programs with pinned maps: %w", err)
	}
	defer closeMapHandles(replacements)
	defer objs.Close()

	links, pins, err := updatePinnedLsmEnforcerLinks(&objs)
	if err == nil {
		replaceLsmEnforcerLinks(links)
		return loadLsmEnforcerRuntimeMaps(pins)
	}
	if !errors.Is(err, errLsmEnforcerPinnedLinksMissing) {
		if len(links) >= expectedLsmEnforcerLinks {
			log.Printf("[LSM-ENFORCER] reused pinned links without program update: %v", err)
			replaceLsmEnforcerLinks(links)
			if loadErr := loadLsmEnforcerRuntimeMaps(pins); loadErr != nil {
				return loadErr
			}
			lsmEnforcer.LastError = fmt.Sprintf("reused pinned links without program update: %v", err)
			return nil
		}
		return err
	}

	links, pins, err = attachLsmEnforcerPrograms(&objs)
	if err != nil {
		return err
	}
	replaceLsmEnforcerLinks(links)
	return loadLsmEnforcerRuntimeMaps(pins)
}

func pinLsmEnforcerMaps(objs *bpf.AgentLsmEnforcerObjects) error {
	for name, m := range map[string]*ebpf.Map{
		"lsm_blocked_exec_paths": objs.LsmBlockedExecPaths,
		"lsm_blocked_exec_names": objs.LsmBlockedExecNames,
		"lsm_blocked_file_names": objs.LsmBlockedFileNames,
		"lsm_enforcer_stats_map": objs.LsmEnforcerStatsMap,
	} {
		if err := m.Pin(filepath.Join(lsmEnforcerMapsDir, name)); err != nil {
			return fmt.Errorf("pin BPF LSM map %s: %w", name, err)
		}
	}
	return nil
}

func loadPinnedLsmEnforcerMaps() (map[string]*ebpf.Map, error) {
	names := []string{"lsm_blocked_exec_paths", "lsm_blocked_exec_names", "lsm_blocked_file_names", "lsm_enforcer_stats_map"}
	maps := make(map[string]*ebpf.Map, len(names))
	for _, name := range names {
		m, err := ebpf.LoadPinnedMap(filepath.Join(lsmEnforcerMapsDir, name), nil)
		if err != nil {
			closeMapHandles(maps)
			return nil, fmt.Errorf("load BPF LSM map %s: %w", name, err)
		}
		maps[name] = m
	}
	return maps, nil
}

func updatePinnedLsmEnforcerLinks(objs *bpf.AgentLsmEnforcerObjects) ([]link.Link, []string, error) {
	specs := []struct {
		name    string
		program *ebpf.Program
	}{
		{name: "bprm_check_security", program: objs.LsmEnforceBprmCheck},
		{name: "file_open", program: objs.LsmEnforceFileOpen},
		{name: "file_permission", program: objs.LsmEnforceFilePermission},
		{name: "mmap_file", program: objs.LsmEnforceMmapFile},
		{name: "file_mprotect", program: objs.LsmEnforceFileMprotect},
		{name: "inode_setattr", program: objs.LsmEnforceInodeSetattr},
		{name: "inode_create", program: objs.LsmEnforceInodeCreate},
		{name: "inode_link", program: objs.LsmEnforceInodeLink},
		{name: "inode_unlink", program: objs.LsmEnforceInodeUnlink},
		{name: "inode_symlink", program: objs.LsmEnforceInodeSymlink},
		{name: "inode_mkdir", program: objs.LsmEnforceInodeMkdir},
		{name: "inode_rmdir", program: objs.LsmEnforceInodeRmdir},
		{name: "inode_mknod", program: objs.LsmEnforceInodeMknod},
		{name: "inode_rename", program: objs.LsmEnforceInodeRename},
	}

	links := make([]link.Link, 0, len(specs))
	pins := make([]string, 0, len(specs))
	for _, spec := range specs {
		pinPath := filepath.Join(lsmEnforcerLinksDir, spec.name)
		lnk, err := link.LoadPinnedLink(pinPath, nil)
		if err != nil {
			for _, opened := range links {
				_ = opened.Close()
			}
			if os.IsNotExist(err) {
				return nil, nil, errLsmEnforcerPinnedLinksMissing
			}
			return nil, nil, fmt.Errorf("load pinned BPF LSM %s link: %w", spec.name, err)
		}
		links = append(links, lnk)
		pins = append(pins, pinPath)
	}

	var updateErrors []string
	for i, spec := range specs {
		if err := links[i].Update(spec.program); err != nil {
			updateErrors = append(updateErrors, fmt.Sprintf("%s: %v", spec.name, err))
		}
	}
	if len(updateErrors) > 0 {
		return links, pins, fmt.Errorf("update pinned BPF LSM links: %s", strings.Join(updateErrors, "; "))
	}
	return links, pins, nil
}

func attachLsmEnforcerPrograms(objs *bpf.AgentLsmEnforcerObjects) ([]link.Link, []string, error) {
	closeLsmEnforcerLinks()
	_ = os.RemoveAll(lsmEnforcerLinksDir)
	if err := os.MkdirAll(lsmEnforcerLinksDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("create BPF LSM links dir: %w", err)
	}

	specs := []struct {
		name    string
		program *ebpf.Program
	}{
		{name: "bprm_check_security", program: objs.LsmEnforceBprmCheck},
		{name: "file_open", program: objs.LsmEnforceFileOpen},
		{name: "file_permission", program: objs.LsmEnforceFilePermission},
		{name: "mmap_file", program: objs.LsmEnforceMmapFile},
		{name: "file_mprotect", program: objs.LsmEnforceFileMprotect},
		{name: "inode_setattr", program: objs.LsmEnforceInodeSetattr},
		{name: "inode_create", program: objs.LsmEnforceInodeCreate},
		{name: "inode_link", program: objs.LsmEnforceInodeLink},
		{name: "inode_unlink", program: objs.LsmEnforceInodeUnlink},
		{name: "inode_symlink", program: objs.LsmEnforceInodeSymlink},
		{name: "inode_mkdir", program: objs.LsmEnforceInodeMkdir},
		{name: "inode_rmdir", program: objs.LsmEnforceInodeRmdir},
		{name: "inode_mknod", program: objs.LsmEnforceInodeMknod},
		{name: "inode_rename", program: objs.LsmEnforceInodeRename},
	}

	links := make([]link.Link, 0, len(specs))
	pins := make([]string, 0, len(specs))
	for _, spec := range specs {
		lnk, err := link.AttachLSM(link.LSMOptions{Program: spec.program})
		if err != nil {
			closeLinksAndRemovePins(links, pins)
			return nil, nil, fmt.Errorf("attach BPF LSM %s: %w", spec.name, err)
		}
		pinPath := filepath.Join(lsmEnforcerLinksDir, spec.name)
		if err := lnk.Pin(pinPath); err != nil {
			log.Printf("[LSM-ENFORCER] attached %s but could not pin link %s: %v; keeping it process-held", spec.name, pinPath, err)
		} else {
			pins = append(pins, pinPath)
		}
		links = append(links, lnk)
	}
	return links, pins, nil
}

func closeLsmEnforcerLinks() {
	for _, existing := range lsmEnforcer.Links {
		_ = existing.Close()
	}
	lsmEnforcer.Links = nil
	lsmEnforcer.LinkPins = nil
}

func replaceLsmEnforcerLinks(links []link.Link) {
	closeLsmEnforcerLinks()
	lsmEnforcer.Links = links
}

func loadLsmEnforcerRuntimeMaps(linkPins []string) error {
	maps, err := loadPinnedLsmEnforcerMaps()
	if err != nil {
		return err
	}

	lsmEnforcer.ExecPathBlocklist = maps["lsm_blocked_exec_paths"]
	lsmEnforcer.ExecNameBlocklist = maps["lsm_blocked_exec_names"]
	lsmEnforcer.FileNameBlocklist = maps["lsm_blocked_file_names"]
	lsmEnforcer.Stats = maps["lsm_enforcer_stats_map"]
	lsmEnforcer.LinkPins = linkPins
	lsmEnforcer.LastError = ""

	log.Printf("[LSM-ENFORCER] active: exec_paths=%v exec_names=%v file_names=%v stats=%v links=%d pinned=%d",
		lsmEnforcer.ExecPathBlocklist != nil,
		lsmEnforcer.ExecNameBlocklist != nil,
		lsmEnforcer.FileNameBlocklist != nil,
		lsmEnforcer.Stats != nil,
		len(lsmEnforcer.Links),
		len(lsmEnforcer.LinkPins))
	return nil
}

func ensureLsmEnforcerMapPermissions() error {
	for _, dir := range []string{lsmEnforcerPinRoot, lsmEnforcerMapsDir, lsmEnforcerLinksDir} {
		if err := os.Chmod(dir, 0755); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("chmod %s: %w", dir, err)
		}
	}
	for _, name := range []string{"lsm_blocked_exec_paths", "lsm_blocked_exec_names", "lsm_blocked_file_names", "lsm_enforcer_stats_map"} {
		path := filepath.Join(lsmEnforcerMapsDir, name)
		// Keep kernel-enforced block policy mutable only through the privileged,
		// authenticated backend API instead of exposing world-writable map pins.
		if err := os.Chmod(path, lsmEnforcerMapPinMode); err != nil {
			return fmt.Errorf("chmod BPF LSM map %s: %w", name, err)
		}
	}
	return nil
}

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
