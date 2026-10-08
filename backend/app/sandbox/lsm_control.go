package sandbox

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"strings"

	"github.com/cilium/ebpf"
)

// ── BPF LSM policy map operations ─────────────────────────────────────

func lsmPathKeyFromString(path string) (lsmPathKey, error) {
	var key lsmPathKey
	normalized, err := NormalizePathString(path)
	if err != nil {
		return key, err
	}
	copy(key.Path[:], normalized)
	return key, nil
}

func NormalizePathString(path string) (string, error) {
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
	normalized, err := NormalizeNameStringWithLabel(name, label)
	if err != nil {
		return key, err
	}
	copy(key.Name[:], normalized)
	return key, nil
}

func NormalizeNameStringWithLabel(name, label string) (string, error) {
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

func BlockExecPath(path string) error { return updateLsmPathBits(path, denyExec, true) }

func UnblockExecPath(path string) error { return updateLsmPathBits(path, denyExec, false) }

func BlockExecName(name string) error {
	key, err := lsmExecNameKeyFromString(name)
	if err != nil {
		return err
	}
	snap := CurrentLsmEnforcerSnapshot()
	if !snap.Available() || !snap.Attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			return err
		}
		snap = CurrentLsmEnforcerSnapshot()
	}
	if snap.ExecNameBlocklist == nil {
		return fmt.Errorf("BPF LSM exec-name blocklist not loaded")
	}
	val := uint32(1)
	return snap.ExecNameBlocklist.Put(&key, &val)
}

func UnblockExecName(name string) error {
	key, err := lsmExecNameKeyFromString(name)
	if err != nil {
		return err
	}
	snap := CurrentLsmEnforcerSnapshot()
	if !snap.Available() || !snap.Attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			return err
		}
		snap = CurrentLsmEnforcerSnapshot()
	}
	if snap.ExecNameBlocklist == nil {
		return fmt.Errorf("BPF LSM exec-name blocklist not loaded")
	}
	return ignoreMissingMapKey(snap.ExecNameBlocklist.Delete(&key))
}

func BlockFileName(name string) error {
	key, err := lsmNameKeyFromString(name)
	if err != nil {
		return err
	}
	snap := CurrentLsmEnforcerSnapshot()
	if !snap.Available() || !snap.Attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			return err
		}
		snap = CurrentLsmEnforcerSnapshot()
	}
	if snap.FileNameBlocklist == nil {
		return fmt.Errorf("BPF LSM enforcer not loaded")
	}
	val := uint32(1)
	return snap.FileNameBlocklist.Put(&key, &val)
}

func UnblockFileName(name string) error {
	key, err := lsmNameKeyFromString(name)
	if err != nil {
		return err
	}
	snap := CurrentLsmEnforcerSnapshot()
	if !snap.Available() || !snap.Attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			return err
		}
		snap = CurrentLsmEnforcerSnapshot()
	}
	if snap.FileNameBlocklist == nil {
		return fmt.Errorf("BPF LSM enforcer not loaded")
	}
	return ignoreMissingMapKey(snap.FileNameBlocklist.Delete(&key))
}

func GetLsmEnforcerStats(statsMap *ebpf.Map) (LsmEnforcerStats, error) {
	if statsMap == nil {
		return LsmEnforcerStats{}, fmt.Errorf("BPF LSM stats map not loaded")
	}

	cpuCount, err := ebpf.PossibleCPU()
	if err != nil || cpuCount <= 0 {
		return LsmEnforcerStats{}, err
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
		return LsmEnforcerStats{}, err
	}

	var total LsmEnforcerStats
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
		if val & denyExec == 0 {
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

// Handler functions moved to app/handlers/lsm_enforcer.go
// Bridge functions in handlersbridge.go delegate to them.

// ListExecPaths returns blocked executable paths.
func ListExecPaths(blocklist *ebpf.Map) []string { return listLsmExecPaths(blocklist) }

// ListExecNames returns blocked executable basenames.
func ListExecNames(blocklist *ebpf.Map) []string { return listLsmExecNames(blocklist) }

// ListFileNames returns blocked file basenames.
func ListFileNames(blocklist *ebpf.Map) []string { return listLsmFileNames(blocklist) }

// NormalizeName validates/normalizes a single name token.
func NormalizeName(name string) (string, error) { return NormalizeNameStringWithLabel(name, "name") }

// The pinned lsm_blocked_exec_paths map retains its key and value ABI.
// bit 0 = deny exec, bit 1 = deny read, bit 2 = deny write.
const (
	denyExec  uint32 = 1
	denyRead  uint32 = 2
	denyWrite uint32 = 4
)
var lsmPathPolicyMu sync.Mutex

type FileAccessRule struct {
	Path string `json:"path"`
	DenyRead bool `json:"denyRead"`
	DenyWrite bool `json:"denyWrite"`
}

func NormalizeFileAccessPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("file restriction requires absolute path")
	}
	path = filepath.Clean(path)
	if path == "/" || len(path) >= 256 {
		return "", fmt.Errorf("invalid path: filesystem root or exceeds 255 bytes")
	}
	if stat, err := os.Stat(path); err == nil && stat.IsDir() {
		return "", fmt.Errorf("this policy protects exact files, not directories")
	}
	return path, nil
}

func mergedPathBits(old, mask uint32, enabled bool) uint32 {
	if enabled { return old | mask }
	return old &^ mask
}

// Serialized read-modify-write prevents updating file bits from erasing an
// existing executable-block bit and vice versa.
func mutatePathBits(path string, update func(uint32) uint32) error {
	key, err := lsmPathKeyFromString(path)
	if err != nil { return err }
	lsmPathPolicyMu.Lock()
	defer lsmPathPolicyMu.Unlock()
	snap := CurrentLsmEnforcerSnapshot()
	if !snap.Available() || !snap.Attached() {
		if err := ensureLsmEnforcerLoaded(); err != nil { return err }
		snap = CurrentLsmEnforcerSnapshot()
	}
	if snap.ExecPathBlocklist == nil { return fmt.Errorf("BPF LSM path map unavailable") }
	var old uint32
	if err := snap.ExecPathBlocklist.Lookup(&key, &old); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		return err
	}
	next := update(old)
	if next == 0 { return ignoreMissingMapKey(snap.ExecPathBlocklist.Delete(&key)) }
	return snap.ExecPathBlocklist.Put(&key, &next)
}

func updateLsmPathBits(path string, mask uint32, enabled bool) error {
	return mutatePathBits(path, func(old uint32) uint32 { return mergedPathBits(old, mask, enabled) })
}

func SetFileAccessPath(path string, denyR, denyW bool) error {
	path, err := NormalizeFileAccessPath(path)
	if err != nil { return err }
	// The file path feature must never appear to succeed when an upgraded
	// kernel enforcer fell back to older pinned programs.
	snap := CurrentLsmEnforcerSnapshot()
	if !snap.PathAccessSupported {
		if err := ensureLsmEnforcerLoaded(); err != nil { return err }
		if !CurrentLsmEnforcerSnapshot().PathAccessSupported {
			return fmt.Errorf("LSM file path access rules are not supported by the attached programs")
		}
	}
	return mutatePathBits(path, func(old uint32) uint32 {
		next := old &^ (denyRead | denyWrite)
		if denyR { next |= denyRead }
		if denyW { next |= denyWrite }
		return next
	})
}

func ListFileAccessPaths(blocklist *ebpf.Map) []FileAccessRule {
	if blocklist == nil { return nil }
	out := []FileAccessRule{}
	iterator := blocklist.Iterate()
	var key lsmPathKey
	var bits uint32
	for iterator.Next(&key, &bits) {
		if bits & (denyRead | denyWrite) == 0 { continue }
		out = append(out, FileAccessRule{
			Path: string(bytes.TrimRight(key.Path[:], "\x00")),
			DenyRead: bits & denyRead != 0,
			DenyWrite: bits & denyWrite != 0,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
