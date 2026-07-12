package app

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/cilium/ebpf"
)

const (
	maxLoadedEBPFPlugins                    = 32
	maxUserBPFPrograms                      = 16
	maxUserBPFMaps                          = 32
	maxUserBPFVariables                     = 128
	maxUserBPFInstructionsPerProgram        = 100_000
	maxUserBPFTotalInstructions             = 250_000
	maxUserBPFMapKeyBytes            uint32 = 4 << 10
	maxUserBPFMapValueBytes          uint32 = 1 << 20
	maxUserBPFMapEntries             uint32 = 1 << 20
	maxUserBPFInitialContents               = 4 << 10
	maxUserBPFMapEstimatedBytes      uint64 = 64 << 20
	maxUserBPFTotalMapBytes          uint64 = 128 << 20
)

var (
	userBPFProgramNamePattern         = regexpMustCompileASCII(`^[A-Za-z_][A-Za-z0-9_.$-]{0,127}$`)
	userBPFTracepointComponentPattern = regexpMustCompileASCII(`^[A-Za-z0-9_.-]{1,128}$`)
	userBPFKprobeTargetPattern        = regexpMustCompileASCII(`^[A-Za-z_][A-Za-z0-9_.$:]{0,255}$`)
	userBPFLSMTargetPattern           = regexpMustCompileASCII(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
)

// regexpMustCompileASCII is kept local to make the accepted kernel-facing
// identifier alphabet explicit at each call site.
func regexpMustCompileASCII(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}

func validateLoadableEBPFPluginManifest(m *PluginManifest) error {
	if m == nil || m.Kind != PluginKindEBPF {
		return errors.New("not an eBPF plugin")
	}
	if err := validatePluginID(m.ID); err != nil {
		return err
	}
	if !userBPFProgramNamePattern.MatchString(strings.TrimSpace(m.ProgramName)) {
		return errors.New("programName must be a valid eBPF symbol of at most 128 bytes")
	}
	switch m.AttachKind {
	case PluginAttachTracepoint:
		_, _, err := splitTracepointTarget(m.AttachTarget)
		return err
	case PluginAttachKprobe, PluginAttachKretprobe:
		if !userBPFKprobeTargetPattern.MatchString(strings.TrimSpace(m.AttachTarget)) {
			return fmt.Errorf("invalid kprobe target %q", m.AttachTarget)
		}
	case PluginAttachLSM:
		target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m.AttachTarget), "lsm/"))
		if target != "" && !userBPFLSMTargetPattern.MatchString(target) {
			return fmt.Errorf("invalid LSM target %q", m.AttachTarget)
		}
	case "", PluginAttachNone:
		return errors.New("attach kind is required")
	default:
		return fmt.Errorf("unsupported attach kind %q", m.AttachKind)
	}
	return nil
}

func validateUserBPFCollectionSpec(spec *ebpf.CollectionSpec, manifest *PluginManifest) (*ebpf.ProgramSpec, error) {
	if spec == nil {
		return nil, errors.New("eBPF collection spec is nil")
	}
	if len(spec.Programs) == 0 || len(spec.Programs) > maxUserBPFPrograms {
		return nil, fmt.Errorf("eBPF object program count %d is outside the allowed range 1..%d", len(spec.Programs), maxUserBPFPrograms)
	}
	if len(spec.Maps) > maxUserBPFMaps {
		return nil, fmt.Errorf("eBPF object map count %d exceeds limit %d", len(spec.Maps), maxUserBPFMaps)
	}
	if len(spec.Variables) > maxUserBPFVariables {
		return nil, fmt.Errorf("eBPF object variable count %d exceeds limit %d", len(spec.Variables), maxUserBPFVariables)
	}

	totalInstructions := 0
	for name, program := range spec.Programs {
		if program == nil {
			return nil, fmt.Errorf("eBPF program %q has a nil spec", name)
		}
		instructions := len(program.Instructions)
		if instructions == 0 || instructions > maxUserBPFInstructionsPerProgram {
			return nil, fmt.Errorf("eBPF program %q instruction count %d is outside the allowed range 1..%d", name, instructions, maxUserBPFInstructionsPerProgram)
		}
		totalInstructions += instructions
		if totalInstructions > maxUserBPFTotalInstructions {
			return nil, fmt.Errorf("eBPF object instruction count exceeds limit %d", maxUserBPFTotalInstructions)
		}
		if program.AttachTarget != nil {
			return nil, fmt.Errorf("eBPF program %q contains a disallowed attach target", name)
		}
	}

	var totalMapBytes uint64
	for name, mapSpec := range spec.Maps {
		estimated, err := validateUserBPFMapSpec(name, mapSpec)
		if err != nil {
			return nil, err
		}
		if totalMapBytes > maxUserBPFTotalMapBytes-estimated {
			return nil, fmt.Errorf("eBPF object estimated map memory exceeds limit %d bytes", maxUserBPFTotalMapBytes)
		}
		totalMapBytes += estimated
	}

	program, ok := spec.Programs[manifest.ProgramName]
	if !ok {
		return nil, fmt.Errorf("program %q not found in object", manifest.ProgramName)
	}
	wantType, err := expectedProgramType(manifest.AttachKind)
	if err != nil {
		return nil, err
	}
	if program.Type != wantType {
		return nil, fmt.Errorf("program %q has type %s, expected %s for %s attachment", manifest.ProgramName, program.Type, wantType, manifest.AttachKind)
	}
	if manifest.AttachKind == PluginAttachLSM {
		requested := strings.TrimPrefix(strings.TrimSpace(manifest.AttachTarget), "lsm/")
		if requested != "" && program.AttachTo != "" && requested != program.AttachTo {
			return nil, fmt.Errorf("LSM target %q does not match object attach target %q", requested, program.AttachTo)
		}
	}
	return program, nil
}

func expectedProgramType(kind PluginAttachKind) (ebpf.ProgramType, error) {
	switch kind {
	case PluginAttachTracepoint:
		return ebpf.TracePoint, nil
	case PluginAttachKprobe, PluginAttachKretprobe:
		return ebpf.Kprobe, nil
	case PluginAttachLSM:
		return ebpf.LSM, nil
	default:
		return ebpf.UnspecifiedProgram, fmt.Errorf("unsupported attach kind %q", kind)
	}
}

func validateUserBPFMapSpec(name string, spec *ebpf.MapSpec) (uint64, error) {
	if spec == nil {
		return 0, fmt.Errorf("eBPF map %q has a nil spec", name)
	}
	if spec.Pinning != ebpf.PinNone {
		return 0, fmt.Errorf("eBPF map %q requests disallowed pinning", name)
	}
	if spec.InnerMap != nil {
		return 0, fmt.Errorf("eBPF map %q uses a disallowed inner map", name)
	}
	if spec.MapExtra != 0 || (spec.Extra != nil && spec.Extra.Len() != 0) {
		return 0, fmt.Errorf("eBPF map %q contains unsupported extra attributes", name)
	}
	if spec.KeySize > maxUserBPFMapKeyBytes || spec.ValueSize > maxUserBPFMapValueBytes {
		return 0, fmt.Errorf("eBPF map %q key/value size %d/%d exceeds limits %d/%d", name, spec.KeySize, spec.ValueSize, maxUserBPFMapKeyBytes, maxUserBPFMapValueBytes)
	}
	if spec.MaxEntries > maxUserBPFMapEntries {
		return 0, fmt.Errorf("eBPF map %q entry count %d exceeds limit %d", name, spec.MaxEntries, maxUserBPFMapEntries)
	}
	if len(spec.Contents) > maxUserBPFInitialContents || (spec.MaxEntries != 0 && uint32(len(spec.Contents)) > spec.MaxEntries) {
		return 0, fmt.Errorf("eBPF map %q has too many initial entries", name)
	}

	entries := uint64(spec.MaxEntries)
	if entries == 0 {
		entries = 1
	}
	var estimated uint64
	if spec.Type == ebpf.RingBuf || spec.Type == ebpf.UserRingbuf {
		estimated = uint64(spec.MaxEntries)
	} else {
		perEntry := uint64(spec.KeySize) + uint64(spec.ValueSize) + 64
		var ok bool
		estimated, ok = checkedUint64Mul(perEntry, entries)
		if !ok {
			return 0, fmt.Errorf("eBPF map %q memory estimate overflow", name)
		}
		if userBPFMapIsPerCPU(spec.Type) {
			estimated, ok = checkedUint64Mul(estimated, uint64(max(runtime.NumCPU(), 1)))
			if !ok {
				return 0, fmt.Errorf("eBPF map %q per-CPU memory estimate overflow", name)
			}
		}
	}
	if estimated > maxUserBPFMapEstimatedBytes {
		return 0, fmt.Errorf("eBPF map %q estimated memory %d exceeds limit %d bytes", name, estimated, maxUserBPFMapEstimatedBytes)
	}
	return estimated, nil
}

func checkedUint64Mul(left, right uint64) (uint64, bool) {
	if left != 0 && right > ^uint64(0)/left {
		return 0, false
	}
	return left * right, true
}

func userBPFMapIsPerCPU(mapType ebpf.MapType) bool {
	switch mapType {
	case ebpf.PerCPUHash, ebpf.PerCPUArray, ebpf.LRUCPUHash, ebpf.PerCPUCGroupStorage:
		return true
	default:
		return false
	}
}
