package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

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
	specs := lsmEnforcerLinkSpecs(objs)

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

	specs := lsmEnforcerLinkSpecs(objs)

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

func lsmEnforcerLinkSpecs(objs *bpf.AgentLsmEnforcerObjects) []struct {
	name    string
	program *ebpf.Program
} {
	return []struct {
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
