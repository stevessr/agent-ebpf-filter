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
// limited to x86_64 SSL_read/SSL_write, matching production TLS discovery.
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
	data, err := os.ReadFile(resolver.path)
	if err != nil {
		resolver.offsetErr = fmt.Errorf("read TLS bpf-ts target %q: %w", resolver.path, err)
		return
	}
	discovery, err := discoverKnownStrippedSSL(resolver.machine, data)
	if err != nil {
		resolver.offsetErr = err
		return
	}
	resolver.offsets["SSL_read"] = discovery.ReadOffset
	resolver.offsets["SSL_write"] = discovery.WriteOffset
}

// discoverKnownStrippedSSLOffsets is retained as a narrow test/helper surface;
// production and bpf-ts resolver behavior both flow through the same discovery
// primitive above.
func discoverKnownStrippedSSLOffsets(data []byte) (map[string]uint64, error) {
	discovery, err := discoverKnownStrippedSSL(elf.EM_X86_64, data)
	if err != nil {
		return nil, err
	}
	return map[string]uint64{
		"SSL_read":  discovery.ReadOffset,
		"SSL_write": discovery.WriteOffset,
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
