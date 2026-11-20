package ebpf

import (
	"testing"

	"github.com/cilium/ebpf"
)

// CollectionSpec parsing needs no privileges. Catch compiler-emitted libc
// calls before NewCollection reaches the kernel (or requires root).
func TestAgentTrackerHasNoUnresolvedCalls(t *testing.T) {
	spec, err := LoadAgentTracker()
	if err != nil {
		t.Fatalf("parse generated tracker: %v", err)
	}
	checkProgramCalls(t, spec)
}

func TestCgroupSandboxCollectionSpec(t *testing.T) {
	spec, err := LoadAgentCgroupSandbox()
	if err != nil {
		t.Fatalf("parse generated cgroup sandbox: %v", err)
	}
	checkProgramCalls(t, spec)

	for _, prog := range []string{
		AgentCgroupSandboxProgCgroupSandboxConnect4,
		AgentCgroupSandboxProgCgroupSandboxConnect6,
		AgentCgroupSandboxProgCgroupSandboxSendmsg4,
		AgentCgroupSandboxProgCgroupSandboxSendmsg6,
	} {
		if _, ok := spec.Programs[prog]; !ok {
			t.Fatalf("missing cgroup program %q", prog)
		}
	}

	const bpfFNoPrealloc = 1
	for _, want := range []struct {
		name       string
		typ        ebpf.MapType
		keySize    uint32
		valSize    uint32
		noPrealloc bool
	}{
		{AgentCgroupSandboxMapCidrBlocklist, ebpf.LPMTrie, 8, 4, true},
		{AgentCgroupSandboxMapCidr6Blocklist, ebpf.LPMTrie, 20, 4, true},
		{AgentCgroupSandboxMapIpBlocklist, ebpf.Hash, 4, 4, false},
		{AgentCgroupSandboxMapIp6Blocklist, ebpf.Hash, 16, 4, false},
		{AgentCgroupSandboxMapPortBlocklist, ebpf.Hash, 4, 4, false},
		{AgentCgroupSandboxMapCgroupBlocklist, ebpf.Hash, 8, 4, false},
	} {
		m, ok := spec.Maps[want.name]
		if !ok {
			t.Fatalf("missing cgroup map %q", want.name)
		}
		if m.Type != want.typ || m.KeySize != want.keySize || m.ValueSize != want.valSize {
			t.Fatalf("map %q spec = (%v, key=%d, val=%d), want (%v, key=%d, val=%d)",
				want.name, m.Type, m.KeySize, m.ValueSize, want.typ, want.keySize, want.valSize)
		}
		if want.noPrealloc && (m.Flags&bpfFNoPrealloc) == 0 {
			t.Fatalf("LPM trie map %q missing BPF_F_NO_PREALLOC flag (flags=%#x)", want.name, m.Flags)
		}
	}
}

func TestLsmEnforcerCollectionSpec(t *testing.T) {
	spec, err := LoadAgentLsmEnforcer()
	if err != nil {
		t.Fatalf("parse generated LSM enforcer: %v", err)
	}
	checkProgramCalls(t, spec)

	for _, prog := range []string{
		AgentLsmEnforcerProgLsmEnforceBprmCheck,
		AgentLsmEnforcerProgLsmEnforceFileOpen,
		AgentLsmEnforcerProgLsmEnforceFilePermission,
		AgentLsmEnforcerProgLsmEnforceMmapFile,
		AgentLsmEnforcerProgLsmEnforceFileMprotect,
		AgentLsmEnforcerProgLsmEnforceInodeSetattr,
		AgentLsmEnforcerProgLsmEnforceInodeCreate,
		AgentLsmEnforcerProgLsmEnforceInodeLink,
		AgentLsmEnforcerProgLsmEnforceInodeUnlink,
		AgentLsmEnforcerProgLsmEnforceInodeSymlink,
		AgentLsmEnforcerProgLsmEnforceInodeMkdir,
		AgentLsmEnforcerProgLsmEnforceInodeRmdir,
		AgentLsmEnforcerProgLsmEnforceInodeMknod,
		AgentLsmEnforcerProgLsmEnforceInodeRename,
	} {
		if _, ok := spec.Programs[prog]; !ok {
			t.Fatalf("missing LSM program %q", prog)
		}
	}

	m, ok := spec.Maps[AgentLsmEnforcerMapLsmBlockedInodes]
	if !ok {
		t.Fatalf("missing LSM map %q", AgentLsmEnforcerMapLsmBlockedInodes)
	}
	if m.Type != ebpf.Hash || m.KeySize != 16 || m.ValueSize != 4 {
		t.Fatalf("map %q spec = (%v, key=%d, val=%d), want (Hash, key=16, val=4)",
			AgentLsmEnforcerMapLsmBlockedInodes, m.Type, m.KeySize, m.ValueSize)
	}
}

func TestIFCEnforcerCollectionSpec(t *testing.T) {
	spec, err := LoadAgentIfcEnforcer()
	if err != nil {
		t.Fatalf("parse generated K-IFC LSM enforcer: %v", err)
	}
	checkProgramCalls(t, spec)

	for _, want := range []struct {
		name     string
		attachTo string
	}{
		{AgentIfcEnforcerProgIfcEnforceTaskAlloc, "task_alloc"},
		{AgentIfcEnforcerProgIfcEnforceFileOpen, "file_open"},
		{AgentIfcEnforcerProgIfcEnforceFilePermission, "file_permission"},
		{AgentIfcEnforcerProgIfcEnforceBprmCheck, "bprm_check_security"},
		{AgentIfcEnforcerProgIfcEnforceInodeUnlink, "inode_unlink"},
		{AgentIfcEnforcerProgIfcEnforceInodeRename, "inode_rename"},
		{AgentIfcEnforcerProgIfcEnforceSocketConnect, "socket_connect"},
		{AgentIfcEnforcerProgIfcEnforceSocketSendmsg, "socket_sendmsg"},
	} {
		prog, ok := spec.Programs[want.name]
		if !ok {
			t.Fatalf("missing K-IFC LSM program %q", want.name)
		}
		if prog.Type != ebpf.LSM || prog.AttachType != ebpf.AttachLSMMac || prog.AttachTo != want.attachTo {
			t.Fatalf("program %q = (%v, %v, attach=%q), want (LSM, AttachLSMMac, %q)",
				want.name, prog.Type, prog.AttachType, prog.AttachTo, want.attachTo)
		}
	}

	const bpfFNoPrealloc = 1
	for _, want := range []struct {
		name       string
		typ        ebpf.MapType
		keySize    uint32
		valSize    uint32
		noPrealloc bool
	}{
		{AgentIfcEnforcerMapIfcTaskLabels, ebpf.TaskStorage, 4, 32, true},
		{AgentIfcEnforcerMapIfcInodeLabels, ebpf.InodeStorage, 4, 16, true},
		{AgentIfcEnforcerMapIfcPidSeed, ebpf.Hash, 4, 32, false},
		{AgentIfcEnforcerMapIfcClassifiedInodes, ebpf.Hash, 16, 16, false},
		{AgentIfcEnforcerMapIfcStatsMap, ebpf.PerCPUArray, 4, 64, false},
	} {
		m, ok := spec.Maps[want.name]
		if !ok {
			t.Fatalf("missing K-IFC map %q", want.name)
		}
		if m.Type != want.typ || m.KeySize != want.keySize || m.ValueSize != want.valSize {
			t.Fatalf("map %q = (%v, key=%d, val=%d), want (%v, key=%d, val=%d)",
				want.name, m.Type, m.KeySize, m.ValueSize, want.typ, want.keySize, want.valSize)
		}
		if want.noPrealloc && (m.Flags&bpfFNoPrealloc) == 0 {
			t.Fatalf("local storage map %q missing BPF_F_NO_PREALLOC (flags=%#x)", want.name, m.Flags)
		}
	}
}

func checkProgramCalls(t *testing.T, spec *ebpf.CollectionSpec) {
	t.Helper()
	for name, program := range spec.Programs {
		t.Run(name, func(t *testing.T) {
			symbols, err := program.Instructions.SymbolOffsets()
			if err != nil {
				t.Fatal(err)
			}
			for i, instruction := range program.Instructions {
				if !instruction.IsFunctionCall() {
					continue
				}
				reference := instruction.Reference()
				if reference == "" {
					continue // Already resolved relative BPF-to-BPF call.
				}
				if _, ok := symbols[reference]; !ok {
					t.Errorf("instruction %d: unresolved function %q", i, reference)
				}
			}
		})
	}
}
