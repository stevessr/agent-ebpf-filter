package agentifc

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"agent-ebpf-filter/ebpf"
)

// Open flag constants matching Linux UAPI / backend/ebpf/ifc_enforcer.c.
const (
	OpenReadOnly  uint32 = 00000000
	OpenWriteOnly uint32 = 00000001
	OpenReadWrite uint32 = 00000002
	OpenTrunc     uint32 = 00001000
	OpenAppend    uint32 = 00002000

	MayWrite uint32 = 2
	MayRead  uint32 = 4
)

// ABIReport captures compile-time struct size verification against bpf2go types.
type ABIReport struct {
	TaskLabelBytes  uintptr `json:"taskLabelBytes"`
	InodeLabelBytes uintptr `json:"inodeLabelBytes"`
	InodeKeyBytes   uintptr `json:"inodeKeyBytes"`
	StatsBytes      uintptr `json:"statsBytes"`
}

// VerifyKernelABI checks that bpf2go struct sizes match the expected kernel C ABI.
func VerifyKernelABI() (ABIReport, error) {
	rep := ABIReport{
		TaskLabelBytes:  unsafe.Sizeof(ebpf.AgentIfcEnforcerIfcTaskLabel{}),
		InodeLabelBytes: unsafe.Sizeof(ebpf.AgentIfcEnforcerIfcInodeLabel{}),
		InodeKeyBytes:   unsafe.Sizeof(ebpf.AgentIfcEnforcerIfcInodeKey{}),
		StatsBytes:      unsafe.Sizeof(ebpf.AgentIfcEnforcerIfcStats{}),
	}
	if rep.TaskLabelBytes != 32 {
		return rep, fmt.Errorf("ifc_task_label size=%d want 32", rep.TaskLabelBytes)
	}
	if rep.InodeLabelBytes != 16 {
		return rep, fmt.Errorf("ifc_inode_label size=%d want 16", rep.InodeLabelBytes)
	}
	if rep.InodeKeyBytes != 16 {
		return rep, fmt.Errorf("ifc_inode_key size=%d want 16", rep.InodeKeyBytes)
	}
	if rep.StatsBytes != 64 {
		return rep, fmt.Errorf("ifc_stats size=%d want 64", rep.StatsBytes)
	}
	return rep, nil
}

// Decision records the synchronous verdict of a K-IFC hook evaluation.
type Decision struct {
	Allowed         bool   `json:"allowed"`
	Errno           int    `json:"errno,omitempty"`
	Hook            string `json:"hook"`
	Reason          string `json:"reason"`
	TargetViolation bool   `json:"targetViolation,omitempty"`
}

type simInodeState struct {
	inodeID uint64
	dyn     ebpf.AgentIfcEnforcerIfcInodeLabel
}

// KernelIFCSimulator provides a bit-exact userspace reference model of the 8-hook
// BPF LSM state machine in backend/ebpf/ifc_enforcer.c (TASK_STORAGE + INODE_STORAGE),
// allowing deterministic verification in unprivileged environments without BPF_PROG_LOAD.
type KernelIFCSimulator struct {
	tasks      map[uint32]*ebpf.AgentIfcEnforcerIfcTaskLabel
	leases     map[uint32]TaskLease
	paths      map[string]uint64 // path -> kernel inode ID (models dentry->d_inode, hardlinks, renames)
	inodes     map[uint64]*simInodeState
	classified map[uint64]ebpf.AgentIfcEnforcerIfcInodeLabel
	nextInode  uint64
	stats      ebpf.AgentIfcEnforcerIfcStats
}

// NewKernelIFCSimulator initializes a fresh K-IFC kernel state simulator.
func NewKernelIFCSimulator() *KernelIFCSimulator {
	return &KernelIFCSimulator{
		tasks:      make(map[uint32]*ebpf.AgentIfcEnforcerIfcTaskLabel),
		leases:     make(map[uint32]TaskLease),
		paths:      make(map[string]uint64),
		inodes:     make(map[uint64]*simInodeState),
		classified: make(map[uint64]ebpf.AgentIfcEnforcerIfcInodeLabel),
		nextInode:  1000,
	}
}

// SeedProcess inserts an initial TaskLease into the simulated task storage for pid.
func (k *KernelIFCSimulator) SeedProcess(pid uint32, lease TaskLease) {
	lease.AuthorizedTargets = normalizeTargets(lease.AuthorizedTargets)
	lbl := lease.ToKernelLabel()
	k.tasks[pid] = &lbl
	k.leases[pid] = lease
}

// NarrowProcess monotonically narrows an active process lease (e.g., on tool entry).
func (k *KernelIFCSimulator) NarrowProcess(pid uint32, req TaskLease) error {
	curLease, ok := k.leases[pid]
	if !ok {
		return fmt.Errorf("pid %d not seeded", pid)
	}
	curLbl := k.tasks[pid]
	// Preserve any runtime taint already accumulated inside kernel TASK_STORAGE.
	curLease.ConfTaint |= curLbl.ConfTaint
	curLease.IntegTaint |= curLbl.IntegTaint
	narrowed, err := NarrowLease(curLease, req)
	if err != nil {
		return err
	}
	lbl := narrowed.ToKernelLabel()
	k.tasks[pid] = &lbl
	k.leases[pid] = narrowed
	return nil
}

// TaskLabel returns a snapshot of a process's kernel TASK_STORAGE label.
func (k *KernelIFCSimulator) TaskLabel(pid uint32) (ebpf.AgentIfcEnforcerIfcTaskLabel, bool) {
	lbl, ok := k.tasks[pid]
	if !ok {
		return ebpf.AgentIfcEnforcerIfcTaskLabel{}, false
	}
	return *lbl, true
}

// ClassifyPath pre-labels an underlying inode in ifc_classified_inodes.
func (k *KernelIFCSimulator) ClassifyPath(path string, confTaint, integTaint uint32) uint64 {
	ino := k.ensureInode(path)
	k.classified[ino] = ebpf.AgentIfcEnforcerIfcInodeLabel{
		ConfTaint:  confTaint,
		IntegTaint: integTaint,
	}
	return ino
}

// InodeLabel returns the dynamic INODE_STORAGE taint attached to path's inode.
func (k *KernelIFCSimulator) InodeLabel(path string) (ebpf.AgentIfcEnforcerIfcInodeLabel, uint64, bool) {
	ino, ok := k.paths[path]
	if !ok {
		return ebpf.AgentIfcEnforcerIfcInodeLabel{}, 0, false
	}
	return k.inodes[ino].dyn, ino, true
}

func (k *KernelIFCSimulator) ensureInode(path string) uint64 {
	if ino, ok := k.paths[path]; ok {
		return ino
	}
	k.nextInode++
	ino := k.nextInode
	k.paths[path] = ino
	k.inodes[ino] = &simInodeState{inodeID: ino}
	return ino
}

func (k *KernelIFCSimulator) readCombinedInodeTaint(ino uint64) (conf, integ uint32) {
	if st, ok := k.inodes[ino]; ok {
		conf |= st.dyn.ConfTaint
		integ |= st.dyn.IntegTaint
	}
	if cls, ok := k.classified[ino]; ok {
		conf |= cls.ConfTaint
		integ |= cls.IntegTaint
	}
	return conf, integ
}

// TaskAlloc mirrors SEC("lsm/task_alloc").
func (k *KernelIFCSimulator) TaskAlloc(parentPID, childPID uint32) Decision {
	parent, ok := k.tasks[parentPID]
	if !ok {
		return Decision{Allowed: true, Hook: "task_alloc", Reason: "untracked_parent"}
	}
	if (parent.Flags&IFCFlagEnforce) != 0 && parent.SpawnBudget == 0 {
		k.stats.SpawnBudgetBlocks++
		return Decision{
			Allowed: false,
			Errno:   int(syscall.EACCES),
			Hook:    "task_alloc",
			Reason:  "spawn_budget_exhausted",
		}
	}
	var childBudget uint32
	if parent.SpawnBudget > 0 {
		parent.SpawnBudget--
		childBudget = parent.SpawnBudget
	}
	child := *parent
	child.SpawnBudget = childBudget
	k.tasks[childPID] = &child
	pl := k.leases[parentPID]
	pl.SpawnBudget = childBudget
	pl.ConfTaint = child.ConfTaint
	pl.IntegTaint = child.IntegTaint
	k.leases[childPID] = pl
	k.stats.TaskInherits++
	return Decision{Allowed: true, Hook: "task_alloc", Reason: "inherited_task_storage"}
}

func (k *KernelIFCSimulator) propagateFileIFC(lbl *ebpf.AgentIfcEnforcerIfcTaskLabel, ino uint64, isRead, isWrite bool, hook string) Decision {
	inConf, inInteg := k.readCombinedInodeTaint(ino)
	if isWrite {
		if (lbl.Flags&IFCFlagEnforce) != 0 &&
			(lbl.IntegTaint&IntegUntrusted) != 0 &&
			(lbl.CapMask&CapFileWrite) == 0 {
			return Decision{
				Allowed: false,
				Errno:   int(syscall.EACCES),
				Hook:    hook,
				Reason:  "untrusted_file_write_denied",
			}
		}
		if lbl.ConfTaint != 0 || lbl.IntegTaint != 0 {
			st := k.inodes[ino]
			st.dyn.ConfTaint |= lbl.ConfTaint
			st.dyn.IntegTaint |= lbl.IntegTaint
			st.dyn.WriterRunHash = lbl.RunIdHash
			k.stats.InodeTaintWrites++
		}
	}
	if isRead {
		if (inConf&ConfSecret) != 0 &&
			(lbl.Flags&IFCFlagEnforce) != 0 &&
			(lbl.CapMask&CapSecretRead) == 0 {
			return Decision{
				Allowed: false,
				Errno:   int(syscall.EACCES),
				Hook:    hook,
				Reason:  "secret_read_without_capability",
			}
		}
		if (inConf & ^lbl.ConfTaint) != 0 || (inInteg & ^lbl.IntegTaint) != 0 {
			lbl.ConfTaint |= inConf
			lbl.IntegTaint |= inInteg
			k.stats.TaskTaintReads++
		}
	}
	return Decision{Allowed: true, Hook: hook, Reason: "ifc_propagated"}
}

// FileOpen mirrors SEC("lsm/file_open").
func (k *KernelIFCSimulator) FileOpen(pid uint32, path string, fflags uint32) Decision {
	lbl, ok := k.tasks[pid]
	if !ok {
		return Decision{Allowed: true, Hook: "file_open", Reason: "untracked_task"}
	}
	ino := k.ensureInode(path)
	isWrite := (fflags & (OpenWriteOnly | OpenReadWrite | OpenTrunc | OpenAppend)) != 0
	isRead := (fflags & OpenWriteOnly) == 0
	return k.propagateFileIFC(lbl, ino, isRead, isWrite, "file_open")
}

// FilePermission mirrors SEC("lsm/file_permission").
func (k *KernelIFCSimulator) FilePermission(pid uint32, path string, mask uint32) Decision {
	lbl, ok := k.tasks[pid]
	if !ok {
		return Decision{Allowed: true, Hook: "file_permission", Reason: "untracked_task"}
	}
	isRead := (mask & MayRead) != 0
	isWrite := (mask & MayWrite) != 0
	if !isRead && !isWrite {
		return Decision{Allowed: true, Hook: "file_permission", Reason: "no_rw_mask"}
	}
	ino := k.ensureInode(path)
	return k.propagateFileIFC(lbl, ino, isRead, isWrite, "file_permission")
}

// BprmCheckSecurity mirrors SEC("lsm/bprm_check_security").
func (k *KernelIFCSimulator) BprmCheckSecurity(pid uint32, path string) Decision {
	lbl, ok := k.tasks[pid]
	if !ok {
		return Decision{Allowed: true, Hook: "bprm_check_security", Reason: "untracked_task"}
	}
	ino := k.ensureInode(path)
	inConf, inInteg := k.readCombinedInodeTaint(ino)
	if (inConf & ^lbl.ConfTaint) != 0 || (inInteg & ^lbl.IntegTaint) != 0 {
		lbl.ConfTaint |= inConf
		lbl.IntegTaint |= inInteg
		k.stats.TaskTaintReads++
	}
	if (lbl.Flags&IFCFlagEnforce) != 0 &&
		((inInteg&IntegUntrusted) != 0 || (lbl.IntegTaint&IntegUntrusted) != 0) &&
		(lbl.CapMask&CapExecUntrusted) == 0 {
		k.stats.UntrustedExecBlocks++
		return Decision{
			Allowed: false,
			Errno:   int(syscall.EACCES),
			Hook:    "bprm_check_security",
			Reason:  "untrusted_binary_exec_denied",
		}
	}
	return Decision{Allowed: true, Hook: "bprm_check_security", Reason: "exec_allowed"}
}

// Hardlink binds dstPath to srcPath's underlying inode identity, modeling link(2).
func (k *KernelIFCSimulator) Hardlink(srcPath, dstPath string) uint64 {
	ino := k.ensureInode(srcPath)
	k.paths[dstPath] = ino
	return ino
}

// InodeUnlink mirrors SEC("lsm/inode_unlink").
func (k *KernelIFCSimulator) InodeUnlink(pid uint32, path string, targetHint string) Decision {
	lbl, ok := k.tasks[pid]
	if !ok {
		delete(k.paths, path)
		return Decision{Allowed: true, Hook: "inode_unlink", Reason: "untracked_task"}
	}
	if (lbl.Flags&IFCFlagEnforce) != 0 &&
		(lbl.IntegTaint&IntegUntrusted) != 0 &&
		(lbl.CapMask&CapFileUnlink) == 0 {
		k.stats.UntrustedUnlinkBlocks++
		return Decision{
			Allowed: false,
			Errno:   int(syscall.EACCES),
			Hook:    "inode_unlink",
			Reason:  "untrusted_unlink_denied",
		}
	}
	if d := k.checkTargetAuthorization(pid, targetHint, "inode_unlink"); !d.Allowed {
		k.stats.UntrustedUnlinkBlocks++
		return d
	}
	delete(k.paths, path)
	return Decision{Allowed: true, Hook: "inode_unlink", Reason: "unlink_allowed"}
}

// InodeRename mirrors SEC("lsm/inode_rename"), preserving inode storage across renames.
func (k *KernelIFCSimulator) InodeRename(pid uint32, oldPath, newPath string, targetHint string) Decision {
	lbl, ok := k.tasks[pid]
	if !ok {
		if ino, exists := k.paths[oldPath]; exists {
			delete(k.paths, oldPath)
			k.paths[newPath] = ino
		}
		return Decision{Allowed: true, Hook: "inode_rename", Reason: "untracked_task"}
	}
	if (lbl.Flags&IFCFlagEnforce) != 0 &&
		(lbl.IntegTaint&IntegUntrusted) != 0 &&
		(lbl.CapMask&CapFileUnlink) == 0 {
		k.stats.UntrustedUnlinkBlocks++
		return Decision{
			Allowed: false,
			Errno:   int(syscall.EACCES),
			Hook:    "inode_rename",
			Reason:  "untrusted_rename_denied",
		}
	}
	if d := k.checkTargetAuthorization(pid, targetHint, "inode_rename"); !d.Allowed {
		k.stats.UntrustedUnlinkBlocks++
		return d
	}
	ino := k.ensureInode(oldPath)
	if lbl.ConfTaint != 0 || lbl.IntegTaint != 0 {
		st := k.inodes[ino]
		st.dyn.ConfTaint |= lbl.ConfTaint
		st.dyn.IntegTaint |= lbl.IntegTaint
		st.dyn.WriterRunHash = lbl.RunIdHash
	}
	delete(k.paths, oldPath)
	k.paths[newPath] = ino
	return Decision{Allowed: true, Hook: "inode_rename", Reason: "rename_allowed"}
}

func (k *KernelIFCSimulator) evaluateSocketEgress(pid uint32, endpoint, targetHint, hook string) Decision {
	if IsLoopbackOrLocalEndpoint(endpoint) {
		return Decision{Allowed: true, Hook: hook, Reason: "loopback_or_local_exempt"}
	}
	lbl, ok := k.tasks[pid]
	if !ok {
		return Decision{Allowed: true, Hook: hook, Reason: "untracked_task"}
	}
	if (lbl.Flags & IFCFlagEnforce) == 0 {
		return Decision{Allowed: true, Hook: hook, Reason: "audit_only"}
	}
	// If TargetBound lease explicitly authorizes this exact destination target
	// (e.g., user explicitly asked "summarize webpage and send DM to Alice"),
	// verify destination binding before enforcing the global Trifecta cut-off;
	// unauthorized destinations when CONF_SECRET + INTEG_UNTRUSTED hit Trifecta first.
	targetAuthorized := k.isTargetAuthorized(pid, targetHint)
	if !targetAuthorized && targetHint != "" && k.isTargetBindingActive(pid) {
		if (lbl.ConfTaint&ConfSecret) != 0 && (lbl.IntegTaint&IntegUntrusted) != 0 {
			k.stats.TrifectaEgressBlocks++
			return Decision{
				Allowed:         false,
				Errno:           int(syscall.EACCES),
				Hook:            hook,
				Reason:          "lethal_trifecta_egress_blocked",
				TargetViolation: true,
			}
		}
		k.stats.UntrustedEgressBlocks++
		return Decision{
			Allowed:         false,
			Errno:           int(syscall.EACCES),
			Hook:            hook,
			Reason:          "target_binding_mismatch",
			TargetViolation: true,
		}
	}

	if (lbl.ConfTaint&ConfSecret) != 0 && (lbl.IntegTaint&IntegUntrusted) != 0 {
		k.stats.TrifectaEgressBlocks++
		return Decision{
			Allowed: false,
			Errno:   int(syscall.EACCES),
			Hook:    hook,
			Reason:  "lethal_trifecta_egress_blocked",
		}
	}
	if (lbl.CapMask&CapNetEgress) == 0 ||
		((lbl.IntegTaint&IntegUntrusted) != 0 && (lbl.CapMask&CapUntrustedEgress) == 0) {
		k.stats.UntrustedEgressBlocks++
		return Decision{
			Allowed: false,
			Errno:   int(syscall.EACCES),
			Hook:    hook,
			Reason:  "untrusted_egress_without_capability",
		}
	}
	return Decision{Allowed: true, Hook: hook, Reason: "egress_allowed"}
}

// SocketConnect mirrors SEC("lsm/socket_connect").
func (k *KernelIFCSimulator) SocketConnect(pid uint32, endpoint string, targetHint string) Decision {
	return k.evaluateSocketEgress(pid, endpoint, targetHint, "socket_connect")
}

// SocketSendmsg mirrors SEC("lsm/socket_sendmsg").
func (k *KernelIFCSimulator) SocketSendmsg(pid uint32, endpoint string, targetHint string) Decision {
	return k.evaluateSocketEgress(pid, endpoint, targetHint, "socket_sendmsg")
}

func (k *KernelIFCSimulator) isTargetBindingActive(pid uint32) bool {
	l, ok := k.leases[pid]
	return ok && l.TargetBound
}

func (k *KernelIFCSimulator) isTargetAuthorized(pid uint32, targetHint string) bool {
	l, ok := k.leases[pid]
	if !ok || !l.TargetBound {
		return true
	}
	cand := normalizeTarget(targetHint)
	if cand == "" {
		return true
	}
	for _, allowed := range l.AuthorizedTargets {
		if cand == allowed || strings.Contains(allowed, cand) || strings.Contains(cand, allowed) {
			return true
		}
	}
	return false
}

func (k *KernelIFCSimulator) checkTargetAuthorization(pid uint32, targetHint, hook string) Decision {
	if targetHint == "" || !k.isTargetBindingActive(pid) {
		return Decision{Allowed: true, Hook: hook}
	}
	if k.isTargetAuthorized(pid, targetHint) {
		return Decision{Allowed: true, Hook: hook}
	}
	return Decision{
		Allowed:         false,
		Errno:           int(syscall.EACCES),
		Hook:            hook,
		Reason:          "target_binding_mismatch",
		TargetViolation: true,
	}
}

// Stats returns accumulated kernel telemetry counters.
func (k *KernelIFCSimulator) Stats() ebpf.AgentIfcEnforcerIfcStats {
	return k.stats
}
