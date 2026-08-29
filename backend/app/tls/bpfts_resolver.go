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

	once       sync.Once
	symbols    map[string]struct{}
	offsets    map[string]uint64
	inspectErr error
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

func (resolver *bpfTSTLSResolver) inspect() {
	resolver.symbols = make(map[string]struct{})
	resolver.offsets = make(map[string]uint64)
	if resolver.path == "" {
		resolver.inspectErr = fmt.Errorf("TLS bpf-ts resolver path is empty")
		return
	}

	file, err := elf.Open(resolver.path)
	if err != nil {
		resolver.inspectErr = fmt.Errorf("open TLS bpf-ts target %q: %w", resolver.path, err)
		return
	}
	machine := file.Machine
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
	_ = file.Close()

	// Exact/heuristic byte patterns in probemanager_offsetstls.go are x86_64
	// machine code. Never apply them to another architecture.
	if machine != elf.EM_X86_64 {
		return
	}
	data, err := os.ReadFile(resolver.path)
	if err != nil {
		resolver.inspectErr = fmt.Errorf("read TLS bpf-ts target %q: %w", resolver.path, err)
		return
	}
	resolved, err := discoverKnownStrippedSSLOffsets(data)
	if err == nil {
		resolver.offsets = resolved
	}
}

func discoverKnownStrippedSSLOffsets(data []byte) (map[string]uint64, error) {
	if !binaryContainsSSLReadWriteStrings(data) {
		return nil, fmt.Errorf("binary does not contain both SSL_read and SSL_write names")
	}

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
	resolver.once.Do(resolver.inspect)
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
		return bpfts.UprobeTarget{
			Path:   resolver.path,
			Symbol: probe.Target,
			PID:    resolver.pid,
		}, nil
	}
	if offset, ok := resolver.offsets[probe.Target]; ok {
		return bpfts.UprobeTarget{
			Path:    resolver.path,
			Address: offset,
			PID:     resolver.pid,
		}, nil
	}
	return bpfts.UprobeTarget{}, fmt.Errorf(
		"TLS bpf-ts target %q has no symbol or safe stripped offset for %q",
		resolver.path,
		probe.Target,
	)
}
