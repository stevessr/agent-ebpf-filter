package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/gin-gonic/gin"
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

// ── Management operations ─────────────────────────────────────────────

func blockCgroup(cgroupID uint64) error {
	snap := currentCgroupSandboxSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureCgroupSandboxLoaded(); err != nil {
			return err
		}
		snap = currentCgroupSandboxSnapshot()
	}
	if snap.CgroupBlocklist == nil {
		return fmt.Errorf("cgroup sandbox not loaded")
	}
	val := uint32(1)
	return snap.CgroupBlocklist.Put(&cgroupID, &val)
}

func unblockCgroup(cgroupID uint64) error {
	snap := currentCgroupSandboxSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureCgroupSandboxLoaded(); err != nil {
			return err
		}
		snap = currentCgroupSandboxSnapshot()
	}
	if snap.CgroupBlocklist == nil {
		return fmt.Errorf("cgroup sandbox not loaded")
	}
	return ignoreMissingMapKey(snap.CgroupBlocklist.Delete(&cgroupID))
}

func cgroupIDForPID(pid int, root string) (uint64, string, error) {
	if pid <= 0 {
		return 0, "", fmt.Errorf("invalid pid: %d", pid)
	}
	if strings.TrimSpace(root) == "" {
		var err error
		root, err = cgroupSandboxAttachPath()
		if err != nil {
			return 0, "", err
		}
	}

	data, err := os.ReadFile(filepath.Join("/proc", fmt.Sprintf("%d", pid), "cgroup"))
	if err != nil {
		return 0, "", fmt.Errorf("read pid %d cgroup: %w", pid, err)
	}
	rel, err := unifiedCgroupRelativePath(data)
	if err != nil {
		return 0, "", fmt.Errorf("pid %d unified cgroup: %w", pid, err)
	}
	cgroupPath, err := resolveCgroupPath(root, rel)
	if err != nil {
		return 0, "", err
	}
	id, err := cgroupIDFromPath(cgroupPath)
	if err != nil {
		return 0, "", err
	}
	return id, cgroupPath, nil
}

func unifiedCgroupRelativePath(data []byte) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) == 3 && parts[0] == "0" && parts[1] == "" {
			if strings.TrimSpace(parts[2]) == "" {
				return "", fmt.Errorf("empty cgroup v2 path")
			}
			return parts[2], nil
		}
	}
	return "", fmt.Errorf("no cgroup v2 entry")
}

func resolveCgroupPath(root, rel string) (string, error) {
	cleanRoot := filepath.Clean(root)
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("empty cgroup path")
	}
	cleanRel := filepath.Clean("/" + strings.TrimPrefix(rel, "/"))
	if cleanRel == "/" {
		return cleanRoot, nil
	}
	return filepath.Join(cleanRoot, strings.TrimPrefix(cleanRel, "/")), nil
}

func cgroupIDFromPath(cgroupPath string) (uint64, error) {
	st, err := os.Stat(cgroupPath)
	if err != nil {
		return 0, fmt.Errorf("stat cgroup path %q: %w", cgroupPath, err)
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("stat cgroup path %q: missing Stat_t", cgroupPath)
	}
	if sys.Ino == 0 {
		return 0, fmt.Errorf("stat cgroup path %q: zero inode", cgroupPath)
	}
	return uint64(sys.Ino), nil
}

type ip6BlockKey struct {
	Addr [4]uint32
}

func ip6BlockKeyFromIP(ip net.IP) (ip6BlockKey, error) {
	var key ip6BlockKey
	ip16 := ip.To16()
	if ip16 == nil || ip.To4() != nil {
		return key, fmt.Errorf("invalid IPv6 address: %s", ip.String())
	}
	for i := 0; i < 4; i++ {
		key.Addr[i] = binary.BigEndian.Uint32(ip16[i*4 : (i+1)*4])
	}
	return key, nil
}

func parseCgroupSandboxIP(ipStr string) (net.IP, string, error) {
	trimmed := strings.TrimSpace(ipStr)
	if trimmed == "" {
		return nil, "", fmt.Errorf("empty IP")
	}
	ip := net.ParseIP(trimmed)
	if ip == nil {
		return nil, "", fmt.Errorf("invalid IP: %s", ipStr)
	}
	return ip, canonicalCgroupSandboxIPText(ip), nil
}

func canonicalCgroupSandboxIPText(ip net.IP) string {
	if ip4 := ip.To4(); ip4 != nil {
		return net.IP(ip4).String()
	}
	return ip.String()
}

func blockIP(ipStr string) error {
	ip, _, err := parseCgroupSandboxIP(ipStr)
	if err != nil {
		return err
	}
	snap := currentCgroupSandboxSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureCgroupSandboxLoaded(); err != nil {
			return err
		}
		snap = currentCgroupSandboxSnapshot()
	}
	val := uint32(1)
	if ip4 := ip.To4(); ip4 != nil {
		if snap.IPBlocklist == nil {
			return fmt.Errorf("cgroup sandbox IPv4 blocklist not loaded")
		}
		ipU32 := binary.BigEndian.Uint32(ip4) // host-order IPv4 key; BPF uses bpf_ntohl(ctx->user_ip4)
		return snap.IPBlocklist.Put(&ipU32, &val)
	}
	key, err := ip6BlockKeyFromIP(ip)
	if err != nil {
		return err
	}
	if snap.IP6Blocklist == nil {
		return fmt.Errorf("cgroup sandbox IPv6 blocklist not loaded")
	}
	return snap.IP6Blocklist.Put(&key, &val)
}

func unblockIP(ipStr string) error {
	ip, _, err := parseCgroupSandboxIP(ipStr)
	if err != nil {
		return err
	}
	snap := currentCgroupSandboxSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureCgroupSandboxLoaded(); err != nil {
			return err
		}
		snap = currentCgroupSandboxSnapshot()
	}
	if ip4 := ip.To4(); ip4 != nil {
		if snap.IPBlocklist == nil {
			return fmt.Errorf("cgroup sandbox IPv4 blocklist not loaded")
		}
		ipU32 := binary.BigEndian.Uint32(ip4)
		return ignoreMissingMapKey(snap.IPBlocklist.Delete(&ipU32))
	}
	key, err := ip6BlockKeyFromIP(ip)
	if err != nil {
		return err
	}
	if snap.IP6Blocklist == nil {
		return fmt.Errorf("cgroup sandbox IPv6 blocklist not loaded")
	}
	return ignoreMissingMapKey(snap.IP6Blocklist.Delete(&key))
}

func blockPort(port uint16) error {
	if err := validateCgroupSandboxPort(port); err != nil {
		return err
	}
	snap := currentCgroupSandboxSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureCgroupSandboxLoaded(); err != nil {
			return err
		}
		snap = currentCgroupSandboxSnapshot()
	}
	if snap.PortBlocklist == nil {
		return fmt.Errorf("cgroup sandbox port blocklist not loaded")
	}
	portU32 := uint32(port)
	val := uint32(1)
	return snap.PortBlocklist.Put(&portU32, &val)
}

func unblockPort(port uint16) error {
	if err := validateCgroupSandboxPort(port); err != nil {
		return err
	}
	snap := currentCgroupSandboxSnapshot()
	if !snap.available() || !snap.attached() {
		if err := ensureCgroupSandboxLoaded(); err != nil {
			return err
		}
		snap = currentCgroupSandboxSnapshot()
	}
	if snap.PortBlocklist == nil {
		return fmt.Errorf("cgroup sandbox port blocklist not loaded")
	}
	portU32 := uint32(port)
	return ignoreMissingMapKey(snap.PortBlocklist.Delete(&portU32))
}

func validateCgroupSandboxPort(port uint16) error {
	if port == 0 {
		return fmt.Errorf("invalid destination port: 0")
	}
	return nil
}

func listBlockedCgroups(blocklist *ebpf.Map) []string {
	if blocklist == nil {
		return nil
	}
	items := []string{}
	iter := blocklist.Iterate()
	var key uint64
	var val uint32
	for iter.Next(&key, &val) {
		if val == 0 {
			continue
		}
		items = append(items, fmt.Sprintf("%d", key))
	}
	sort.Slice(items, func(i, j int) bool {
		if len(items[i]) == len(items[j]) {
			return items[i] < items[j]
		}
		return len(items[i]) < len(items[j])
	})
	return items
}

func listBlockedIPs(blocklist, ip6Blocklist *ebpf.Map) []string {
	items := []string{}
	if blocklist != nil {
		iter := blocklist.Iterate()
		var key uint32
		var val uint32
		for iter.Next(&key, &val) {
			if val == 0 {
				continue
			}
			items = append(items, ipv4StringFromBlockKey(key))
		}
	}
	if ip6Blocklist != nil {
		iter := ip6Blocklist.Iterate()
		var key ip6BlockKey
		var val uint32
		for iter.Next(&key, &val) {
			if val == 0 {
				continue
			}
			items = append(items, ip6StringFromBlockKey(key))
		}
	}
	sort.Strings(items)
	return items
}

func ipv4StringFromBlockKey(key uint32) string {
	return net.IPv4(byte(key>>24), byte(key>>16), byte(key>>8), byte(key)).String()
}

func ip6StringFromBlockKey(key ip6BlockKey) string {
	var raw [16]byte
	for i, part := range key.Addr {
		binary.BigEndian.PutUint32(raw[i*4:(i+1)*4], part)
	}
	return net.IP(raw[:]).String()
}

func listBlockedPorts(blocklist *ebpf.Map) []uint16 {
	if blocklist == nil {
		return nil
	}
	items := []uint16{}
	iter := blocklist.Iterate()
	var key uint32
	var val uint32
	for iter.Next(&key, &val) {
		if val == 0 || key == 0 || key > 65535 {
			continue
		}
		items = append(items, uint16(key))
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	return items
}

type cgroupIDRequest struct {
	CgroupID json.RawMessage `json:"cgroupId"`
}

func parseCgroupID(raw json.RawMessage) (uint64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, fmt.Errorf("missing cgroupId")
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		id, parseErr := strconv.ParseUint(strings.TrimSpace(asString), 10, 64)
		if parseErr != nil || id == 0 {
			return 0, fmt.Errorf("invalid cgroupId: %s", asString)
		}
		return id, nil
	}

	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err != nil {
		return 0, fmt.Errorf("invalid cgroupId")
	}
	id, err := strconv.ParseUint(asNumber.String(), 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid cgroupId: %s", asNumber.String())
	}
	return id, nil
}

// ── Statistics ────────────────────────────────────────────────────────

type cgroupSandboxStats struct {
	ConnectChecked uint64 `json:"connectChecked"`
	ConnectBlocked uint64 `json:"connectBlocked"`
	ConnectAllowed uint64 `json:"connectAllowed"`
	Checked        uint64 `json:"checked"`
	Blocked        uint64 `json:"blocked"`
	Allowed        uint64 `json:"allowed"`
}

func getCgroupSandboxStats(statsMap *ebpf.Map) (cgroupSandboxStats, error) {
	if statsMap == nil {
		return cgroupSandboxStats{}, fmt.Errorf("stats map not loaded")
	}

	cpuCount, err := ebpf.PossibleCPU()
	if err != nil || cpuCount <= 0 {
		return cgroupSandboxStats{}, err
	}

	type rawStats struct {
		ConnectChecked uint64
		ConnectBlocked uint64
		ConnectAllowed uint64
	}

	values := make([]rawStats, cpuCount)
	key := uint32(0)
	if err := statsMap.Lookup(&key, &values); err != nil {
		return cgroupSandboxStats{}, err
	}

	var total cgroupSandboxStats
	for _, s := range values {
		total.ConnectChecked += s.ConnectChecked
		total.ConnectBlocked += s.ConnectBlocked
		total.ConnectAllowed += s.ConnectAllowed
	}
	total.Checked = total.ConnectChecked
	total.Blocked = total.ConnectBlocked
	total.Allowed = total.ConnectAllowed
	return total, nil
}

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

// Apply cgroup sandbox to a specific agent run (block all outbound for that cgroup)
func sandboxCgroupForAgent(cgroupID uint64) error {
	if currentCgroupSandboxSnapshot().CgroupBlocklist == nil {
		return fmt.Errorf("cgroup sandbox not available")
	}
	return blockCgroup(cgroupID)
}

// Release cgroup sandbox (allow outbound again)
func releaseCgroupSandbox(cgroupID uint64) error {
	if currentCgroupSandboxSnapshot().CgroupBlocklist == nil {
		return nil
	}
	return unblockCgroup(cgroupID)
}
