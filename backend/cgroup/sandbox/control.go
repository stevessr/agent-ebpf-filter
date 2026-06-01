package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

// ── Cgroup sandbox eBPF map management ────────────────────────────────

type cgroupSandboxMaps struct {
	CgroupBlocklist *ebpf.Map
	IPBlocklist     *ebpf.Map
	IP6Blocklist    *ebpf.Map
	PortBlocklist   *ebpf.Map
	SandboxStats    *ebpf.Map
	Links           []link.Link
	LinkPins        []string
	CgroupPath      string
	LastError       string
}

var cgroupSandbox cgroupSandboxMaps
var cgroupSandboxMu sync.RWMutex
var errCgroupSandboxPinnedLinksMissing = errors.New("cgroup sandbox pinned links missing")

const cgroupSandboxPinRoot = ebpfPinRoot + "/cgroup_sandbox"
const cgroupSandboxMapsDir = cgroupSandboxPinRoot + "/maps"
const cgroupSandboxLinksDir = cgroupSandboxPinRoot + "/links"
const cgroupSandboxMapPinMode os.FileMode = 0600
const cgroup2SuperMagic = 0x63677270

func ensureCgroupSandboxLoaded() error {
	cgroupSandboxMu.Lock()
	defer cgroupSandboxMu.Unlock()

	if cgroupSandboxAvailableLocked() && cgroupSandboxAttachedLocked() {
		return nil
	}

	cgroupPath, err := cgroupSandboxAttachPath()
	if err != nil {
		cgroupSandbox.LastError = err.Error()
		return err
	}

	if err := ensureCgroupSandboxPinnedMapCompatibility(); err != nil {
		cgroupSandbox.LastError = err.Error()
		return err
	}
	if pinnedMaps, err := loadPinnedCgroupSandboxMaps(); err == nil {
		closeMapHandles(pinnedMaps)
		if err := attachCgroupSandboxWithPinnedMaps(cgroupPath); err != nil {
			// Preserve existing pinned policy maps when reattach fails. Deleting
			// them here would silently erase active OS-level block policy just
			// because a restarted backend lacks privileges or the host rejects a
			// new attach attempt.
			cgroupSandbox.LastError = err.Error()
			return err
		}
		return nil
	}

	if err := bootstrapCgroupSandbox(cgroupPath); err != nil {
		cgroupSandbox.LastError = err.Error()
		return err
	}
	cgroupSandbox.LastError = ""
	return nil
}

func cgroupSandboxAvailable() bool {
	cgroupSandboxMu.RLock()
	defer cgroupSandboxMu.RUnlock()
	return cgroupSandboxAvailableLocked()
}

func cgroupSandboxAvailableLocked() bool {
	return cgroupSandbox.CgroupBlocklist != nil &&
		cgroupSandbox.IPBlocklist != nil &&
		cgroupSandbox.IP6Blocklist != nil &&
		cgroupSandbox.PortBlocklist != nil &&
		cgroupSandbox.SandboxStats != nil
}

func cgroupSandboxAttached() bool {
	cgroupSandboxMu.RLock()
	defer cgroupSandboxMu.RUnlock()
	return cgroupSandboxAttachedLocked()
}

func cgroupSandboxAttachedLocked() bool {
	return len(cgroupSandbox.Links) >= 4
}

type cgroupSandboxSnapshot struct {
	CgroupBlocklist *ebpf.Map
	IPBlocklist     *ebpf.Map
	IP6Blocklist    *ebpf.Map
	PortBlocklist   *ebpf.Map
	SandboxStats    *ebpf.Map
	LinkCount       int
	LinkPins        []string
	CgroupPath      string
	LastError       string
}

func currentCgroupSandboxSnapshot() cgroupSandboxSnapshot {
	cgroupSandboxMu.RLock()
	defer cgroupSandboxMu.RUnlock()
	return cgroupSandboxSnapshot{
		CgroupBlocklist: cgroupSandbox.CgroupBlocklist,
		IPBlocklist:     cgroupSandbox.IPBlocklist,
		IP6Blocklist:    cgroupSandbox.IP6Blocklist,
		PortBlocklist:   cgroupSandbox.PortBlocklist,
		SandboxStats:    cgroupSandbox.SandboxStats,
		LinkCount:       len(cgroupSandbox.Links),
		LinkPins:        append([]string(nil), cgroupSandbox.LinkPins...),
		CgroupPath:      cgroupSandbox.CgroupPath,
		LastError:       cgroupSandbox.LastError,
	}
}

func (s cgroupSandboxSnapshot) available() bool {
	return s.CgroupBlocklist != nil && s.IPBlocklist != nil && s.IP6Blocklist != nil && s.PortBlocklist != nil && s.SandboxStats != nil
}

func (s cgroupSandboxSnapshot) attached() bool {
	return s.LinkCount >= 4
}

func cgroupSandboxAttachPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("AGENT_CGROUP_SANDBOX_PATH")); p != "" {
		if err := validateCgroupSandboxAttachPath(p); err != nil {
			return "", err
		}
		return p, nil
	}

	for _, p := range []string{"/sys/fs/cgroup"} {
		if err := validateCgroupSandboxAttachPath(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("cgroup v2 mount not found; set AGENT_CGROUP_SANDBOX_PATH")
}

func validateCgroupSandboxAttachPath(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cgroup sandbox attach path %q: %w", path, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("cgroup sandbox attach path %q is not a directory", path)
	}

	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return fmt.Errorf("statfs cgroup sandbox attach path %q: %w", path, err)
	}
	if uint64(fs.Type) != cgroup2SuperMagic {
		return fmt.Errorf("cgroup sandbox attach path %q is not on a cgroup v2 filesystem", path)
	}

	if _, err := os.Stat(filepath.Join(path, "cgroup.procs")); err != nil {
		return fmt.Errorf("cgroup sandbox attach path %q is not a cgroup directory: %w", path, err)
	}
	return nil
}

func bootstrapCgroupSandbox(cgroupPath string) error {
	_ = os.RemoveAll(cgroupSandboxPinRoot)
	for _, d := range []string{cgroupSandboxMapsDir, cgroupSandboxLinksDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}

	var objs bpf.AgentCgroupSandboxObjects
	if err := bpf.LoadAgentCgroupSandboxObjects(&objs, nil); err != nil {
		return fmt.Errorf("load cgroup sandbox eBPF objects: %w", err)
	}
	defer objs.Close()

	if err := pinCgroupSandboxMaps(&objs); err != nil {
		return err
	}
	if err := ensureCgroupSandboxMapPermissions(); err != nil {
		return err
	}
	links, pins, err := attachCgroupSandboxPrograms(&objs, cgroupPath)
	if err != nil {
		return err
	}
	replaceCgroupSandboxLinks(links)
	if err := loadCgroupSandboxRuntimeMaps(cgroupPath, pins); err != nil {
		return err
	}
	return nil
}

func attachCgroupSandboxWithPinnedMaps(cgroupPath string) error {
	if err := ensureCgroupSandboxPinnedMapCompatibility(); err != nil {
		return err
	}
	replacements, err := loadPinnedCgroupSandboxMaps()
	if err != nil {
		return err
	}
	if err := ensureCgroupSandboxMapPermissions(); err != nil {
		closeMapHandles(replacements)
		return err
	}

	var objs bpf.AgentCgroupSandboxObjects
	if err := bpf.LoadAgentCgroupSandboxObjects(&objs, &ebpf.CollectionOptions{MapReplacements: replacements}); err != nil {
		closeMapHandles(replacements)
		return fmt.Errorf("load cgroup sandbox programs with pinned maps: %w", err)
	}
	defer closeMapHandles(replacements)
	defer objs.Close()

	links, pins, err := updatePinnedCgroupSandboxLinks(&objs)
	if err == nil {
		replaceCgroupSandboxLinks(links)
		return loadCgroupSandboxRuntimeMaps(cgroupPath, pins)
	}
	if !errors.Is(err, errCgroupSandboxPinnedLinksMissing) {
		if len(links) >= 4 {
			log.Printf("[CGROUP-SANDBOX] reused pinned links without program update: %v", err)
			replaceCgroupSandboxLinks(links)
			if loadErr := loadCgroupSandboxRuntimeMaps(cgroupPath, pins); loadErr != nil {
				return loadErr
			}
			cgroupSandbox.LastError = fmt.Sprintf("reused pinned links without program update: %v", err)
			return nil
		}
		return err
	}

	links, pins, err = attachCgroupSandboxPrograms(&objs, cgroupPath)
	if err != nil {
		return err
	}
	replaceCgroupSandboxLinks(links)
	return loadCgroupSandboxRuntimeMaps(cgroupPath, pins)
}

func pinCgroupSandboxMaps(objs *bpf.AgentCgroupSandboxObjects) error {
	for name, m := range map[string]*ebpf.Map{
		"cgroup_blocklist":     objs.CgroupBlocklist,
		"ip_blocklist":         objs.IpBlocklist,
		"ip6_blocklist":        objs.Ip6Blocklist,
		"port_blocklist":       objs.PortBlocklist,
		"cgroup_sandbox_stats": objs.CgroupSandboxStats,
	} {
		if err := m.Pin(filepath.Join(cgroupSandboxMapsDir, name)); err != nil {
			return fmt.Errorf("pin cgroup sandbox map %s: %w", name, err)
		}
	}
	return nil
}

func loadPinnedCgroupSandboxMaps() (map[string]*ebpf.Map, error) {
	names := []string{"cgroup_blocklist", "ip_blocklist", "ip6_blocklist", "port_blocklist", "cgroup_sandbox_stats"}
	maps := make(map[string]*ebpf.Map, len(names))
	for _, name := range names {
		m, err := ebpf.LoadPinnedMap(filepath.Join(cgroupSandboxMapsDir, name), nil)
		if err != nil {
			closeMapHandles(maps)
			return nil, fmt.Errorf("load cgroup sandbox map %s: %w", name, err)
		}
		maps[name] = m
	}
	return maps, nil
}

func ensureCgroupSandboxPinnedMapCompatibility() error {
	if _, err := os.Stat(cgroupSandboxMapsDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat cgroup sandbox maps dir: %w", err)
	}

	ip6Path := filepath.Join(cgroupSandboxMapsDir, "ip6_blocklist")
	if _, err := os.Stat(ip6Path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat cgroup sandbox IPv6 blocklist map: %w", err)
	}

	m, err := ebpf.NewMap(&ebpf.MapSpec{
		Name:       "ip6_blocklist",
		Type:       ebpf.Hash,
		KeySize:    16,
		ValueSize:  4,
		MaxEntries: 1024,
	})
	if err != nil {
		return fmt.Errorf("create missing cgroup sandbox IPv6 blocklist map: %w", err)
	}
	defer m.Close()
	if err := m.Pin(ip6Path); err != nil {
		return fmt.Errorf("pin missing cgroup sandbox IPv6 blocklist map: %w", err)
	}
	if err := os.Chmod(ip6Path, cgroupSandboxMapPinMode); err != nil {
		return fmt.Errorf("chmod missing cgroup sandbox IPv6 blocklist map: %w", err)
	}
	return nil
}

func updatePinnedCgroupSandboxLinks(objs *bpf.AgentCgroupSandboxObjects) ([]link.Link, []string, error) {
	specs := []struct {
		name    string
		program *ebpf.Program
	}{
		{name: "connect4", program: objs.CgroupSandboxConnect4},
		{name: "connect6", program: objs.CgroupSandboxConnect6},
		{name: "sendmsg4", program: objs.CgroupSandboxSendmsg4},
		{name: "sendmsg6", program: objs.CgroupSandboxSendmsg6},
	}

	links := make([]link.Link, 0, len(specs))
	pins := make([]string, 0, len(specs))
	for _, spec := range specs {
		pinPath := filepath.Join(cgroupSandboxLinksDir, spec.name)
		lnk, err := link.LoadPinnedLink(pinPath, nil)
		if err != nil {
			for _, opened := range links {
				_ = opened.Close()
			}
			if os.IsNotExist(err) {
				return nil, nil, errCgroupSandboxPinnedLinksMissing
			}
			return nil, nil, fmt.Errorf("load pinned cgroup/%s link: %w", spec.name, err)
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
		return links, pins, fmt.Errorf("update pinned cgroup sandbox links: %s", strings.Join(updateErrors, "; "))
	}
	return links, pins, nil
}

func attachCgroupSandboxPrograms(objs *bpf.AgentCgroupSandboxObjects, cgroupPath string) ([]link.Link, []string, error) {
	closeCgroupSandboxLinks()
	_ = os.RemoveAll(cgroupSandboxLinksDir)
	if err := os.MkdirAll(cgroupSandboxLinksDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("create cgroup sandbox links dir: %w", err)
	}

	specs := []struct {
		name    string
		attach  ebpf.AttachType
		program *ebpf.Program
	}{
		{name: "connect4", attach: ebpf.AttachCGroupInet4Connect, program: objs.CgroupSandboxConnect4},
		{name: "connect6", attach: ebpf.AttachCGroupInet6Connect, program: objs.CgroupSandboxConnect6},
		{name: "sendmsg4", attach: ebpf.AttachCGroupUDP4Sendmsg, program: objs.CgroupSandboxSendmsg4},
		{name: "sendmsg6", attach: ebpf.AttachCGroupUDP6Sendmsg, program: objs.CgroupSandboxSendmsg6},
	}

	links := make([]link.Link, 0, len(specs))
	pins := make([]string, 0, len(specs))
	for _, spec := range specs {
		lnk, err := link.AttachCgroup(link.CgroupOptions{
			Path:    cgroupPath,
			Attach:  spec.attach,
			Program: spec.program,
		})
		if err != nil {
			closeLinksAndRemovePins(links, pins)
			return nil, nil, fmt.Errorf("attach cgroup/%s at %s: %w", spec.name, cgroupPath, err)
		}

		pinPath := filepath.Join(cgroupSandboxLinksDir, spec.name)
		if err := lnk.Pin(pinPath); err != nil {
			log.Printf("[CGROUP-SANDBOX] attached cgroup/%s but could not pin link %s: %v; keeping it process-held", spec.name, pinPath, err)
		} else {
			pins = append(pins, pinPath)
		}
		links = append(links, lnk)
	}
	return links, pins, nil
}

func closeLinksAndRemovePins(links []link.Link, pins []string) {
	for _, opened := range links {
		_ = opened.Close()
	}
	for _, pin := range pins {
		_ = os.Remove(pin)
	}
}

func ignoreMissingMapKey(err error) error {
	if errors.Is(err, ebpf.ErrKeyNotExist) {
		return nil
	}
	return err
}

func closeCgroupSandboxLinks() {
	for _, existing := range cgroupSandbox.Links {
		_ = existing.Close()
	}
	cgroupSandbox.Links = nil
	cgroupSandbox.LinkPins = nil
}

func replaceCgroupSandboxLinks(links []link.Link) {
	closeCgroupSandboxLinks()
	cgroupSandbox.Links = links
}

func loadCgroupSandboxRuntimeMaps(cgroupPath string, linkPins []string) error {
	maps, err := loadPinnedCgroupSandboxMaps()
	if err != nil {
		return err
	}

	cgroupSandbox.CgroupBlocklist = maps["cgroup_blocklist"]
	cgroupSandbox.IPBlocklist = maps["ip_blocklist"]
	cgroupSandbox.IP6Blocklist = maps["ip6_blocklist"]
	cgroupSandbox.PortBlocklist = maps["port_blocklist"]
	cgroupSandbox.SandboxStats = maps["cgroup_sandbox_stats"]
	cgroupSandbox.CgroupPath = cgroupPath
	cgroupSandbox.LinkPins = linkPins
	cgroupSandbox.LastError = ""

	log.Printf("[CGROUP-SANDBOX] active on %s: cgroup=%v ip=%v ip6=%v port=%v stats=%v links=%d pinned=%d",
		cgroupPath,
		cgroupSandbox.CgroupBlocklist != nil,
		cgroupSandbox.IPBlocklist != nil,
		cgroupSandbox.IP6Blocklist != nil,
		cgroupSandbox.PortBlocklist != nil,
		cgroupSandbox.SandboxStats != nil,
		len(cgroupSandbox.Links),
		len(cgroupSandbox.LinkPins))
	return nil
}

func ensureCgroupSandboxMapPermissions() error {
	for _, dir := range []string{cgroupSandboxPinRoot, cgroupSandboxMapsDir, cgroupSandboxLinksDir} {
		if err := os.Chmod(dir, 0755); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("chmod %s: %w", dir, err)
		}
	}
	for _, name := range []string{"cgroup_blocklist", "ip_blocklist", "ip6_blocklist", "port_blocklist", "cgroup_sandbox_stats"} {
		path := filepath.Join(cgroupSandboxMapsDir, name)
		// Keep OS-level enforcement maps writable only by the privileged backend.
		// Unlike agent registration maps, these policy maps should not be
		// mutated directly by unprivileged adapters or local users.
		if err := os.Chmod(path, cgroupSandboxMapPinMode); err != nil {
			return fmt.Errorf("chmod cgroup sandbox map %s: %w", name, err)
		}
	}
	return nil
}
