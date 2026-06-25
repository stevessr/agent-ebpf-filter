package sandbox

import (
	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"
)

// Manager aggregates all sandbox state previously held in package-level global
// variables. Create via NewManager and pass to dependent code.
//
// The Manager delegates to package-level functions that operate on the
// package-level globals (cgroupSandbox, lsmEnforcer). This is an incremental
// step: existing code continues working, and new code injects the Manager.
// A future phase will thread *Manager through the delegate functions directly.
type Manager struct{}

// NewManager creates a Manager.
func NewManager() *Manager {
	return &Manager{}
}

// ── Cgroup sandbox methods ──────────────────────────────────────────────────

func (m *Manager) EnsureCgroupSandboxLoaded() error        { return ensureCgroupSandboxLoaded() }
func (m *Manager) CgroupSandboxAvailable() bool            { return cgroupSandboxAvailable() }
func (m *Manager) CgroupSandboxAttached() bool             { return cgroupSandboxAttached() }
func (m *Manager) CurrentCgroupSandboxSnapshot() cgroupSandboxSnapshot { return currentCgroupSandboxSnapshot() }
func (m *Manager) BlockCgroup(cgroupID uint64) error       { return blockCgroup(cgroupID) }
func (m *Manager) UnblockCgroup(cgroupID uint64) error     { return unblockCgroup(cgroupID) }
func (m *Manager) BlockIP(ipStr string) error               { return blockIP(ipStr) }
func (m *Manager) UnblockIP(ipStr string) error             { return unblockIP(ipStr) }
func (m *Manager) BlockPort(port uint16) error              { return blockPort(port) }
func (m *Manager) UnblockPort(port uint16) error            { return unblockPort(port) }

// ── LSM enforcer methods ───────────────────────────────────────────────────

func (m *Manager) EnsureLsmEnforcerLoaded() error           { return ensureLsmEnforcerLoaded() }
func (m *Manager) LsmEnforcerAvailable() bool               { return lsmEnforcerAvailable() }
func (m *Manager) LsmEnforcerAttached() bool                { return lsmEnforcerAttached() }
func (m *Manager) CurrentLsmEnforcerSnapshot() lsmEnforcerSnapshot { return currentLsmEnforcerSnapshot() }
func (m *Manager) BlockLsmExecPath(path string) error       { return blockLsmExecPath(path) }
func (m *Manager) UnblockLsmExecPath(path string) error     { return unblockLsmExecPath(path) }
func (m *Manager) BlockLsmExecName(name string) error       { return blockLsmExecName(name) }
func (m *Manager) UnblockLsmExecName(name string) error     { return unblockLsmExecName(name) }
func (m *Manager) BlockLsmFileName(name string) error       { return blockLsmFileName(name) }
func (m *Manager) UnblockLsmFileName(name string) error     { return unblockLsmFileName(name) }

// ── HTTP handlers ──────────────────────────────────────────────────────────

func (m *Manager) HandleCgroupSandboxStatus(c *gin.Context)       { handleCgroupSandboxStatus(c) }
func (m *Manager) HandleCgroupSandboxBlockCgroup(c *gin.Context)  { handleCgroupSandboxBlockCgroup(c) }
func (m *Manager) HandleCgroupSandboxUnblockCgroup(c *gin.Context){ handleCgroupSandboxUnblockCgroup(c) }
func (m *Manager) HandleCgroupSandboxBlockPID(c *gin.Context)     { handleCgroupSandboxBlockPID(c) }
func (m *Manager) HandleCgroupSandboxUnblockPID(c *gin.Context)   { handleCgroupSandboxUnblockPID(c) }
func (m *Manager) HandleCgroupSandboxBlockIP(c *gin.Context)      { handleCgroupSandboxBlockIP(c) }
func (m *Manager) HandleCgroupSandboxUnblockIP(c *gin.Context)    { handleCgroupSandboxUnblockIP(c) }
func (m *Manager) HandleCgroupSandboxBlockPort(c *gin.Context)    { handleCgroupSandboxBlockPort(c) }
func (m *Manager) HandleCgroupSandboxUnblockPort(c *gin.Context)  { handleCgroupSandboxUnblockPort(c) }
func (m *Manager) HandleLsmEnforcerStatus(c *gin.Context)         { handleLsmEnforcerStatus(c) }
func (m *Manager) HandleLsmBlockExecPath(c *gin.Context)          { handleLsmBlockExecPath(c) }
func (m *Manager) HandleLsmUnblockExecPath(c *gin.Context)        { handleLsmUnblockExecPath(c) }
func (m *Manager) HandleLsmBlockExecName(c *gin.Context)          { handleLsmBlockExecName(c) }
func (m *Manager) HandleLsmUnblockExecName(c *gin.Context)        { handleLsmUnblockExecName(c) }
func (m *Manager) HandleLsmBlockFileName(c *gin.Context)          { handleLsmBlockFileName(c) }
func (m *Manager) HandleLsmUnblockFileName(c *gin.Context)        { handleLsmUnblockFileName(c) }

// ── Stateless helpers (exposed for convenience) ─────────────────────────────

func GetCgroupSandboxStats(statsMap *ebpf.Map) (cgroupSandboxStats, error) {
	return getCgroupSandboxStats(statsMap)
}

func GetLsmEnforcerStats(statsMap *ebpf.Map) (lsmEnforcerStats, error) {
	return getLsmEnforcerStats(statsMap)
}

func ListBlockedCgroups(bl *ebpf.Map) []string           { return listBlockedCgroups(bl) }
func ListBlockedIPs(bl, bl6 *ebpf.Map) []string          { return listBlockedIPs(bl, bl6) }
func ListBlockedPorts(bl *ebpf.Map) []uint16             { return listBlockedPorts(bl) }
func ListLsmExecPaths(bl *ebpf.Map) []string             { return listLsmExecPaths(bl) }
func ListLsmExecNames(bl *ebpf.Map) []string             { return listLsmExecNames(bl) }
func ListLsmFileNames(bl *ebpf.Map) []string             { return listLsmFileNames(bl) }
func CgroupIDForPID(pid int, root string) (uint64, string, error) { return cgroupIDForPID(pid, root) }
func SandboxCgroupForAgent(cid uint64) error              { return sandboxCgroupForAgent(cid) }
func ReleaseCgroupSandbox(cid uint64) error                { return releaseCgroupSandbox(cid) }