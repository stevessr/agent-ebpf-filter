package app

// Bridge aliases onto package sandbox (see backend/app/sandbox).

import "agent-ebpf-filter/app/sandbox"

var (
	ensureCgroupSandboxLoaded = sandbox.EnsureLoaded
	ensureLsmEnforcerLoaded   = sandbox.EnsureLSMLoaded

	currentCgroupSandboxSnapshot = sandbox.CurrentCgroupSandboxSnapshot
	currentLsmEnforcerSnapshot   = sandbox.CurrentLsmEnforcerSnapshot
	getCgroupSandboxStats        = sandbox.GetCgroupSandboxStats
	getLsmEnforcerStats          = sandbox.GetLsmEnforcerStats
	listBlockedCgroups           = sandbox.ListBlockedCgroups
	listBlockedIPs               = sandbox.ListBlockedIPs
	listBlockedPorts             = sandbox.ListBlockedPorts
	listLsmExecPaths             = sandbox.ListExecPaths
	listLsmExecNames             = sandbox.ListExecNames
	listLsmFileNames             = sandbox.ListFileNames
	blockCgroup                  = sandbox.BlockCgroup
	unblockCgroup                = sandbox.UnblockCgroup
	blockIP                      = sandbox.BlockIP
	unblockIP                    = sandbox.UnblockIP
	blockPort                    = sandbox.BlockPort
	unblockPort                  = sandbox.UnblockPort
	parseCgroupSandboxIP         = sandbox.ParseIP
	validateCgroupSandboxPort    = sandbox.ValidatePort
	cgroupIDForPID               = sandbox.CgroupIDForPID
	normalizeLsmPathString       = sandbox.NormalizePathString
	parseSandboxCgroupID         = sandbox.ParseID
	normalizeLsmNameString       = sandbox.NormalizeName

	blockLsmExecPath   = sandbox.BlockExecPath
	unblockLsmExecPath = sandbox.UnblockExecPath
	blockLsmExecName   = sandbox.BlockExecName
	unblockLsmExecName = sandbox.UnblockExecName
	blockLsmFileName   = sandbox.BlockFileName
	unblockLsmFileName = sandbox.UnblockFileName
)
