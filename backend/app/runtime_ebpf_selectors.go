package app

import (
	"agent-ebpf-filter/app/platform"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/cilium/ebpf"
)

const (
	trackingModePathExact uint32 = 1 << iota
	trackingModePathPrefix
)

func trackingModeFlags(hasExact, hasPrefix bool) uint32 {
	var flags uint32
	if hasExact {
		flags |= trackingModePathExact
	}
	if hasPrefix {
		flags |= trackingModePathPrefix
	}
	return flags
}

func mapHasEntries(m *ebpf.Map) (bool, error) {
	if m == nil {
		return false, errors.New("map is nil")
	}
	key, err := m.NextKeyBytes(nil)
	if err != nil {
		if errors.Is(err, ebpf.ErrKeyNotExist) {
			return false, nil
		}
		return false, err
	}
	return len(key) != 0, nil
}

func syncTrackingModeMap(mode, paths, prefixes *ebpf.Map) error {
	if mode == nil || paths == nil || prefixes == nil {
		return errors.New("tracking mode maps are incomplete")
	}
	hasExact, err := mapHasEntries(paths)
	if err != nil {
		return fmt.Errorf("inspect exact path tracking map: %w", err)
	}
	hasPrefix, err := mapHasEntries(prefixes)
	if err != nil {
		return fmt.Errorf("inspect prefix path tracking map: %w", err)
	}
	flags := trackingModeFlags(hasExact, hasPrefix)
	key := uint32(0)
	if err := mode.Update(&key, &flags, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("update tracking mode flags: %w", err)
	}
	return nil
}

func syncTrackingModeFlags(set *trackerMapSet) error {
	if set == nil {
		return errors.New("tracker map set is nil")
	}
	return syncTrackingModeMap(set.TrackingMode, set.TrackedPaths, set.TrackedPrefixes)
}

var trackingModeMutationMu sync.Mutex

func forceTrackingModePathAny(mode *ebpf.Map) error {
	if mode == nil {
		return errors.New("tracking mode map is nil")
	}
	flags := trackingModePathExact | trackingModePathPrefix
	key := uint32(0)
	if err := mode.Update(&key, &flags, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("force fail-open tracking mode: %w", err)
	}
	return nil
}

func rollbackTrackedSelector(set *trackerMapSet, target *ebpf.Map, key any, hadPrevious bool, previous uint32, syncErr error) error {
	var rollbackErr error
	if hadPrevious {
		rollbackErr = target.Put(key, previous)
	} else if err := target.Delete(key); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		rollbackErr = err
	}
	// A sync failure must never leave the kernel in a false-negative state.
	// If exact reconstruction fails for any reason, force both path classes on;
	// extra path work is preferable to suppressing a configured rule.
	failOpenErr := forceTrackingModePathAny(set.TrackingMode)
	if rollbackErr != nil || failOpenErr != nil {
		return errors.Join(
			fmt.Errorf("sync path tracking mode: %w", syncErr),
			fmt.Errorf("rollback tracked selector: %w", rollbackErr),
			failOpenErr,
		)
	}
	return fmt.Errorf("sync path tracking mode: %w", syncErr)
}

func putTrackedSelector(set *trackerMapSet, target *ebpf.Map, key, value any) error {
	if set == nil || target == nil {
		return errors.New("tracked selector maps are incomplete")
	}
	trackingModeMutationMu.Lock()
	defer trackingModeMutationMu.Unlock()

	var previous uint32
	hadPrevious := false
	if err := target.Lookup(key, &previous); err == nil {
		hadPrevious = true
	} else if !errors.Is(err, ebpf.ErrKeyNotExist) {
		return fmt.Errorf("snapshot tracked selector: %w", err)
	}
	if err := target.Put(key, value); err != nil {
		return err
	}
	if err := syncTrackingModeFlags(set); err != nil {
		return rollbackTrackedSelector(set, target, key, hadPrevious, previous, err)
	}
	return nil
}

func deleteTrackedSelector(set *trackerMapSet, target *ebpf.Map, key any) error {
	if set == nil || target == nil {
		return errors.New("tracked selector maps are incomplete")
	}
	trackingModeMutationMu.Lock()
	defer trackingModeMutationMu.Unlock()

	var previous uint32
	if err := target.Lookup(key, &previous); err != nil {
		return err
	}
	if err := target.Delete(key); err != nil {
		return err
	}
	if err := syncTrackingModeFlags(set); err != nil {
		return rollbackTrackedSelector(set, target, key, true, previous, err)
	}
	return nil
}

// ── tracked data preservation ─────────────────────────────────────────────────

type trackedDataBackup struct {
	Comms    map[[16]byte]uint32
	Paths    map[[256]byte]uint32
	Prefixes []prefixBackupEntry
}

type prefixBackupEntry struct {
	Data      [64]byte
	PrefixLen uint32
	Value     uint32
}

func extractTrackedData(maps map[string]*ebpf.Map) *trackedDataBackup {
	b := &trackedDataBackup{
		Comms: make(map[[16]byte]uint32),
		Paths: make(map[[256]byte]uint32),
	}

	if m, ok := maps["tracked_comms"]; ok && m != nil {
		iter := m.Iterate()
		var k [16]byte
		var v uint32
		for iter.Next(&k, &v) {
			b.Comms[k] = v
		}
	}

	if m, ok := maps["tracked_paths"]; ok && m != nil {
		iter := m.Iterate()
		var k [256]byte
		var v uint32
		for iter.Next(&k, &v) {
			b.Paths[k] = v
		}
	}

	if m, ok := maps["tracked_prefixes"]; ok && m != nil {
		iter := m.Iterate()
		for {
			var k struct {
				PrefixLen uint32
				Data      [64]byte
			}
			var v uint32
			if !iter.Next(&k, &v) {
				break
			}
			b.Prefixes = append(b.Prefixes, prefixBackupEntry{
				Data:      k.Data,
				PrefixLen: k.PrefixLen,
				Value:     v,
			})
		}
	}

	return b
}

func restoreTrackedDataToMaps(backup *trackedDataBackup, comms, paths, prefixes *ebpf.Map) error {
	if backup == nil {
		return nil
	}
	if comms == nil || paths == nil || prefixes == nil {
		return errors.New("tracked map restore target is incomplete")
	}
	for k, v := range backup.Comms {
		if err := comms.Put(k, v); err != nil {
			return fmt.Errorf("restore tracked comm: %w", err)
		}
	}
	for k, v := range backup.Paths {
		if err := paths.Put(k, v); err != nil {
			return fmt.Errorf("restore tracked path: %w", err)
		}
	}
	for _, entry := range backup.Prefixes {
		k := struct {
			PrefixLen uint32
			Data      [64]byte
		}{PrefixLen: entry.PrefixLen, Data: entry.Data}
		if err := prefixes.Put(k, entry.Value); err != nil {
			return fmt.Errorf("restore tracked prefix: %w", err)
		}
	}
	return nil
}

func extractTrackedDataBestEffort() *trackedDataBackup {
	maps := make(map[string]*ebpf.Map, 3)
	for _, name := range []string{"tracked_comms", "tracked_paths", "tracked_prefixes"} {
		m, err := ebpf.LoadPinnedMap(filepath.Join(ebpfPinMapsDir, name), nil)
		if err == nil {
			maps[name] = m
		}
	}
	if len(maps) == 0 {
		return nil
	}
	defer platform.CloseMapHandles(maps)
	return extractTrackedData(maps)
}

// ── shared helpers ────────────────────────────────────────────────────────────

func loadPinnedMapHandles() (map[string]*ebpf.Map, error) {
	out := make(map[string]*ebpf.Map, len(mapNames))
	for _, n := range mapNames {
		m, err := ebpf.LoadPinnedMap(filepath.Join(ebpfPinMapsDir, n), nil)
		if err != nil {
			platform.CloseMapHandles(out)
			return nil, err
		}
		out[n] = m
	}
	return out, nil
}

func closeTrackerMapSet(set *trackerMapSet) {
	if set == nil {
		return
	}
	for _, mp := range []*(*ebpf.Map){
		&set.AgentPids, &set.Events, &set.CollectorStats, &set.ContextPressureStats,
		&set.TrackedComms, &set.TrackedPaths, &set.TrackedPrefixes, &set.TrackingMode,
		&set.ExitCtx, &set.ExitIoCtx, &set.ExitCompactCtx, &set.ExitSinglePathBuf, &set.ExitPathBuf,
		&set.ExitSinglePathCtx, &set.ExitPathCtx, &set.SocketFds, &set.SocketFdParents,
	} {
		if *mp != nil {
			_ = (*mp).Close()
			*mp = nil
		}
	}
}
