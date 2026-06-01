package app

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/cilium/ebpf"
)

// ---- moved from backend/zz_merged_backend.go section cgroupsandboxops.go ----

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
