package sandbox

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cilium/ebpf"
)

// ---- moved from backend/zz_merged_backend.go section lsmenforcercontrol.go ----

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

func BlockExecPath(path string) error {
	key, err := lsmPathKeyFromString(path)
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
	if snap.ExecPathBlocklist == nil {
		return fmt.Errorf("BPF LSM enforcer not loaded")
	}
	val := uint32(1)
	return snap.ExecPathBlocklist.Put(&key, &val)
}

func UnblockExecPath(path string) error {
	key, err := lsmPathKeyFromString(path)
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
	if snap.ExecPathBlocklist == nil {
		return fmt.Errorf("BPF LSM enforcer not loaded")
	}
	return ignoreMissingMapKey(snap.ExecPathBlocklist.Delete(&key))
}

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
