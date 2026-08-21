// Package sandbox owns the cgroup-network and BPF-LSM enforcement objects:
// loading, pinning, attach/detach, blocklist mutation, and stats.
package sandbox

import "agent-ebpf-filter/app/platform"

// Pin roots derived from the platform constant; package-level vars so the
// const-concatenation sites keep working.
var (
	ebpfPinRoot = platform.EBPFPinRoot

	cgroupSandboxPinRoot = platform.EBPFPinRoot + "/cgroup_sandbox"
	lsmEnforcerPinRoot   = platform.EBPFPinRoot + "/lsm_enforcer"
)
