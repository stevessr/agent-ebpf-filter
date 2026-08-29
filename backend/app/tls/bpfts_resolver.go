package tls

import (
	"debug/elf"
	"fmt"
	"os"
	"sync"

	"agent-ebpf-filter/app/bpfts"
)

type bpfTSTLSResolver struct {
	path string
	pid  int

	inspectOnce sync.Once
	machine     elf.Machine
	symbols     map[string]struct{}
	inspectErr  error

	offsetOnce sync.Once
	offsets    map[string]uint64
	offsetErr  error
}

// NewBpfTSUprobeResolver adapts the TLS subsystem's executable knowledge to the
// generic bpfts loader. The caller should pass the same executable or map_files
// path that TLS auto-discovery selected. Symbol attachment is preferred on every
// architecture. A stripped-binary byte-pattern fallback is intentionally
// limited to x86_64 SSL_read/SSL_write, matching the existing production
// BoringSSL/OpenSSL fallback's fail-closed architecture assumptions.
func NewBpfTSUprobeResolver(path string, pid int) bpfts.UprobeResolver {
	resolver := &bpfTSTLSResolver{path: path, pid: pid}
	return resolver.resolve
}

func (resolver *bpfTSTLSResolver) inspectELF() {
	resolver.symbols = make(map[string]struct{})
	if resolver.path == "" {
		resolver.inspectErr = fmt.Errorf("TLS bpf-ts resolver path is empty")
		return
	}

	file, err := elf.Open(resolver.path)
	if err != nil {
		resolver.inspectErr = fmt.Errorf("open TLS bpf-ts target %q: %w", resolver.path, err)
		return
	}
	defer file.Close()
	resolver.machine = file.Machine
	for _, readSymbols := range []func() ([]elf.Symbol, error){file.Symbols, file.DynamicSymbols} {
		symbols, symbolErr := readSymbols()
		if symbolErr != nil {
			continue
		}
		for _, symbol := range symbols {
			if symbol.Name != "" {
				resolver.symbols[symbol.Name] = struct{}{}
			}
		}
	}
}

func isKnownStrippedSSLTarget(target string) bool {
	return target == "SSL_read" || target == "SSL_write"
}

func (resolver *bpfTSTLSResolver) inspectOffsets() {
	resolver.offsets = make(map[string]uint64)
	// Exact/heuristic byte patterns in probemanager_offsetstls.go are x86_64
	// machine code. Never apply them to another architecture.
	if resolver.machine != elf.EM_X86_64 {
		resolver.offsetErr = fmt.Errorf("safe stripped SSL offsets are unavailable for ELF machine %s", resolver.machine)
		return
	}
	data, err := os.ReadFile(resolver.path)
	if err != nil {
		resolver.offsetErr = fmt.Errorf("read TLS bpf-ts target %q: %w", resolver.path, err)
		return
	}
	resolver.offsets, resolver.offsetErr = discoverKnownStrippedSSLOffsets(data)
}

func discoverKnownStrippedSSLOffsets(data []byte) (map[string]uint64, error) {
	// Exact BoringSSL machine-code signatures are stronger evidence than a
	// surviving symbol-name string. Accept them even when aggressive stripping
	// removed SSL_read/SSL_write strings from the binary.
	readOff := findBS(data, bsSSLRead.pattern)
	if readOff >= 0 {
		writeCenter := readOff + 0xCA0
		writeOff := findBSNear(data, bsSSLWrite.pattern, writeCenter, 0x10000)
		if writeOff >= 0 {
			return map[string]uint64{
				"SSL_read":  uint64(readOff),
				"SSL_write": uint64(writeOff),
			}, nil
		}
	}

	// The OpenSSL common prologue is intentionally weak and compiler-generated,
	// so its fallback remains gated by both function-name strings and the
	// existing exact-two-candidate distance check.
	if !binaryContainsSSLReadWriteStrings(data) {
		return nil, fmt.Errorf("no exact BoringSSL match and binary does not contain both SSL_read and SSL_write names")
	}
	osslMask := buildMask(osslSSLCommonPrefix.pattern)
	matches := make([]int64, 0, 2)
	for i := int64(0); i <= int64(len(data))-int64(len(osslSSLCommonPrefix.pattern)); i++ {
		if matchMasked(data[i:], osslSSLCommonPrefix.pattern, osslMask) {
			matches = append(matches, i)
		}
	}
	writeOff, readOff, _, ok := selectUnambiguousOpenSSLPair(matches, true)
	if !ok {
		return nil, fmt.Errorf("no unambiguous stripped SSL_read/SSL_write offsets")
	}
	return map[string]uint64{
		"SSL_read":  uint64(readOff),
		"SSL_write": uint64(writeOff),
	}, nil
}

func (resolver *bpfTSTLSResolver) resolve(probe bpfts.ManifestProbe) (bpfts.UprobeTarget, error) {
	resolver.inspectOnce.Do(resolver.inspectELF)
	if resolver.inspectErr != nil {
		return bpfts.UprobeTarget{}, resolver.inspectErr
	}
	if probe.Kind != "uprobe" && probe.Kind != "uretprobe" {
		return bpfts.UprobeTarget{}, fmt.Errorf("TLS bpf-ts resolver cannot resolve probe kind %q", probe.Kind)
	}
	if probe.Target == "" {
		return bpfts.UprobeTarget{}, fmt.Errorf("TLS bpf-ts probe %q has an empty target", probe.Name)
	}
	if _, ok := resolver.symbols[probe.Target]; ok {
		return bpfts.UprobeTarget{Path: resolver.path, Symbol: probe.Target, PID: resolver.pid}, nil
	}

	// Avoid reading/scanning a potentially large executable for arbitrary symbol
	// misses. The stripped fallback currently has signatures only for these two
	// OpenSSL/BoringSSL functions.
	if !isKnownStrippedSSLTarget(probe.Target) {
		return bpfts.UprobeTarget{}, fmt.Errorf(
			"TLS bpf-ts target %q has no ELF symbol for %q and no safe stripped resolver is registered for it",
			resolver.path,
			probe.Target,
		)
	}
	resolver.offsetOnce.Do(resolver.inspectOffsets)
	if offset, ok := resolver.offsets[probe.Target]; ok {
		return bpfts.UprobeTarget{Path: resolver.path, Address: offset, PID: resolver.pid}, nil
	}
	if resolver.offsetErr != nil {
		return bpfts.UprobeTarget{}, fmt.Errorf(
			"TLS bpf-ts target %q could not safely resolve stripped %q: %w",
			resolver.path,
			probe.Target,
			resolver.offsetErr,
		)
	}
	return bpfts.UprobeTarget{}, fmt.Errorf(
		"TLS bpf-ts target %q has no symbol or safe stripped offset for %q",
		resolver.path,
		probe.Target,
	)
}
