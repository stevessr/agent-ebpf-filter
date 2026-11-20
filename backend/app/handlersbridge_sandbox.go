package app

import (
	"agent-ebpf-filter/app/handlers"

	"github.com/cilium/ebpf"
)

// ── Cgroup sandbox adapter ─────────────────────────────────────────

// cgroupSandboxAdapter wraps app-level cgroup sandbox functions to implement handlers.CgroupSandboxOps.
type cgroupSandboxAdapter struct{}

func (a *cgroupSandboxAdapter) Snapshot() handlers.CgroupSandboxSnapshot {
	snap := currentCgroupSandboxSnapshot()
	return handlers.CgroupSandboxSnapshot{
		Available:       snap.Available(),
		Attached:        snap.Attached(),
		CgroupPath:      snap.CgroupPath,
		LinkCount:       snap.LinkCount,
		LinkPins:        snap.LinkPins,
		LastError:       snap.LastError,
		CgroupBlocklist: snap.CgroupBlocklist,
		IPBlocklist:     snap.IPBlocklist,
		IP6Blocklist:    snap.IP6Blocklist,
		PortBlocklist:   snap.PortBlocklist,
		SandboxStats:    snap.SandboxStats,
	}
}

func (a *cgroupSandboxAdapter) EnsureLoaded() error { return ensureCgroupSandboxLoaded() }

func (a *cgroupSandboxAdapter) GetStats(statsMap any) (map[string]any, error) {
	cgStats, err := getCgroupSandboxStats(statsMap.(*ebpf.Map))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"connectChecked": cgStats.ConnectChecked,
		"connectBlocked": cgStats.ConnectBlocked,
		"connectAllowed": cgStats.ConnectAllowed,
		"checked":        cgStats.Checked,
		"blocked":        cgStats.Blocked,
		"allowed":        cgStats.Allowed,
	}, nil
}

func (a *cgroupSandboxAdapter) ListBlockedCgroups(blocklist any) []string {
	return listBlockedCgroups(blocklist.(*ebpf.Map))
}

func (a *cgroupSandboxAdapter) ListBlockedIPs(ipBlocklist, ip6Blocklist any) []string {
	return listBlockedIPs(ipBlocklist.(*ebpf.Map), ip6Blocklist.(*ebpf.Map))
}

func (a *cgroupSandboxAdapter) ListBlockedPorts(portBlocklist any) []uint16 {
	return listBlockedPorts(portBlocklist.(*ebpf.Map))
}

func (a *cgroupSandboxAdapter) BlockCgroup(cgroupID uint64) error   { return blockCgroup(cgroupID) }
func (a *cgroupSandboxAdapter) UnblockCgroup(cgroupID uint64) error { return unblockCgroup(cgroupID) }

func (a *cgroupSandboxAdapter) CgroupIDForPID(pid int, cgroupPath string) (uint64, string, error) {
	return cgroupIDForPID(pid, cgroupPath)
}

func (a *cgroupSandboxAdapter) ParseIP(ip string) (bool, string, error) {
	_, ipText, err := parseCgroupSandboxIP(ip)
	return false, ipText, err // first return is unused in handlers
}

func (a *cgroupSandboxAdapter) ParseCgroupID(raw string) (uint64, error) {
	return parseCgroupIDStr(raw)
}

func (a *cgroupSandboxAdapter) ValidatePort(port uint16) error {
	return validateCgroupSandboxPort(port)
}
func (a *cgroupSandboxAdapter) BlockIP(ip string) error       { return blockIP(ip) }
func (a *cgroupSandboxAdapter) UnblockIP(ip string) error     { return unblockIP(ip) }
func (a *cgroupSandboxAdapter) BlockPort(port uint16) error   { return blockPort(port) }
func (a *cgroupSandboxAdapter) UnblockPort(port uint16) error { return unblockPort(port) }

func parseCgroupIDStr(raw string) (uint64, error) { return parseSandboxCgroupID(raw) }

// ── LSM enforcer adapter ────────────────────────────────────────────

// lsmEnforcerAdapter wraps app-level LSM enforcer functions to implement handlers.LsmEnforcerOps.
type lsmEnforcerAdapter struct{}

func (a *lsmEnforcerAdapter) Snapshot() handlers.LsmEnforcerSnapshot {
	snap := currentLsmEnforcerSnapshot()
	return handlers.LsmEnforcerSnapshot{
		Available:           snap.Available(),
		Attached:            snap.Attached(),
		LinkCount:           snap.LinkCount,
		LinkPins:            snap.LinkPins,
		LastError:           snap.LastError,
		PathAccessSupported: snap.PathAccessSupported,
		ExecPathBlocklist:   snap.ExecPathBlocklist,
		ExecNameBlocklist:   snap.ExecNameBlocklist,
		FileNameBlocklist:   snap.FileNameBlocklist,
		Stats:               snap.Stats,
	}
}

func (a *lsmEnforcerAdapter) EnsureLoaded() error { return ensureLsmEnforcerLoaded() }

func (a *lsmEnforcerAdapter) GetStats(statsMap any) (map[string]any, error) {
	lsmStats, err := getLsmEnforcerStats(statsMap.(*ebpf.Map))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"execChecked": lsmStats.ExecChecked,
		"execBlocked": lsmStats.ExecBlocked,
		"fileChecked": lsmStats.FileChecked,
		"fileBlocked": lsmStats.FileBlocked,
	}, nil
}

func (a *lsmEnforcerAdapter) ListExecPaths(blocklist any) []string {
	return listLsmExecPaths(blocklist.(*ebpf.Map))
}

func (a *lsmEnforcerAdapter) ListExecNames(blocklist any) []string {
	return listLsmExecNames(blocklist.(*ebpf.Map))
}

func (a *lsmEnforcerAdapter) ListFileNames(blocklist any) []string {
	return listLsmFileNames(blocklist.(*ebpf.Map))
}

func (a *lsmEnforcerAdapter) ListFileAccessPaths(m any) []handlers.LsmFileAccessRule {
	if m == nil {
		return nil
	}
	rows := listLsmFileAccessPaths(m.(*ebpf.Map))
	out := make([]handlers.LsmFileAccessRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, handlers.LsmFileAccessRule{
			Path: row.Path, DenyRead: row.DenyRead, DenyWrite: row.DenyWrite,
		})
	}
	return out
}

func (a *lsmEnforcerAdapter) SetFileAccessPath(path string, read, write bool) error {
	return setLsmFileAccessPath(path, read, write)
}

func (a *lsmEnforcerAdapter) NormalizeFileAccessPath(path string) (string, error) {
	return normalizeLsmFileAccessPath(path)
}

func (a *lsmEnforcerAdapter) NormalizePath(path string) (string, error) {
	return normalizeLsmPathString(path)
}

func (a *lsmEnforcerAdapter) NormalizeName(name string) (string, error) {
	return normalizeLsmNameString(name)
}

func (a *lsmEnforcerAdapter) BlockExecPath(path string) error   { return blockLsmExecPath(path) }
func (a *lsmEnforcerAdapter) UnblockExecPath(path string) error { return unblockLsmExecPath(path) }
func (a *lsmEnforcerAdapter) BlockExecName(name string) error   { return blockLsmExecName(name) }
func (a *lsmEnforcerAdapter) UnblockExecName(name string) error { return unblockLsmExecName(name) }
func (a *lsmEnforcerAdapter) BlockFileName(name string) error   { return blockLsmFileName(name) }
func (a *lsmEnforcerAdapter) UnblockFileName(name string) error { return unblockLsmFileName(name) }
