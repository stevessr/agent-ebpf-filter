package tls

import (
	"debug/elf"
	"debug/gosym"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type goTLSProbeTarget struct {
	Name    string
	Address uint64
}

type goTLSTargetCacheEntry struct {
	size        int64
	modUnixNano int64
	targets     []goTLSProbeTarget
	err         string
}

var goTLSTargetCache sync.Map

func cloneGoTLSTargets(targets []goTLSProbeTarget) []goTLSProbeTarget {
	return append([]goTLSProbeTarget(nil), targets...)
}

func virtualAddressToFileOffset(exe *elf.File, address uint64) (uint64, bool) {
	if exe == nil || address == 0 {
		return 0, false
	}
	for _, program := range exe.Progs {
		if program == nil || program.Type != elf.PT_LOAD || program.Filesz == 0 {
			continue
		}
		if address < program.Vaddr || address >= program.Vaddr+program.Filesz {
			continue
		}
		return program.Off + (address - program.Vaddr), true
	}
	return 0, false
}

func appendGoTLSTarget(targets []goTLSProbeTarget, seen map[string]struct{}, name string, address uint64) []goTLSProbeTarget {
	canonical, ok := goTLSSymbolName(name)
	if !ok || address == 0 {
		return targets
	}
	if _, exists := seen[canonical]; exists {
		return targets
	}
	seen[canonical] = struct{}{}
	return append(targets, goTLSProbeTarget{Name: canonical, Address: address})
}

func goTLSTargetsFromELFSymbols(exe *elf.File) []goTLSProbeTarget {
	if exe == nil {
		return nil
	}
	symbols, err := exe.Symbols()
	if err != nil {
		symbols, _ = exe.DynamicSymbols()
	}
	seen := make(map[string]struct{}, 2)
	targets := make([]goTLSProbeTarget, 0, 2)
	for _, symbol := range symbols {
		if _, ok := goTLSSymbolName(symbol.Name); !ok {
			continue
		}
		address, ok := virtualAddressToFileOffset(exe, symbol.Value)
		if !ok {
			continue
		}
		targets = appendGoTLSTarget(targets, seen, symbol.Name, address)
	}
	return targets
}

func goTLSTargetsFromPCLN(exe *elf.File) []goTLSProbeTarget {
	if exe == nil {
		return nil
	}
	pclnSection := exe.Section(".gopclntab")
	textSection := exe.Section(".text")
	if pclnSection == nil || textSection == nil {
		return nil
	}
	pcln, err := pclnSection.Data()
	if err != nil || len(pcln) == 0 {
		return nil
	}
	lineTable := gosym.NewLineTable(pcln, textSection.Addr)
	table, err := gosym.NewTable(nil, lineTable)
	if err != nil || table == nil {
		return nil
	}
	seen := make(map[string]struct{}, 2)
	targets := make([]goTLSProbeTarget, 0, 2)
	for _, function := range table.Funcs {
		if _, ok := goTLSSymbolName(function.Name); !ok {
			continue
		}
		address, ok := virtualAddressToFileOffset(exe, function.Entry)
		if !ok {
			continue
		}
		targets = appendGoTLSTarget(targets, seen, function.Name, address)
	}
	return targets
}

func parseGoTLSTargets(binPath string) ([]goTLSProbeTarget, error) {
	info, err := os.Stat(binPath)
	if err != nil {
		return nil, err
	}
	cacheKey := filepath.Clean(binPath)
	if cached, ok := goTLSTargetCache.Load(cacheKey); ok {
		entry := cached.(goTLSTargetCacheEntry)
		if entry.size == info.Size() && entry.modUnixNano == info.ModTime().UnixNano() {
			if entry.err != "" {
				return nil, fmt.Errorf("%s", entry.err)
			}
			return cloneGoTLSTargets(entry.targets), nil
		}
	}

	exe, err := elf.Open(binPath)
	if err != nil {
		goTLSTargetCache.Store(cacheKey, goTLSTargetCacheEntry{size: info.Size(), modUnixNano: info.ModTime().UnixNano(), err: err.Error()})
		return nil, err
	}
	defer exe.Close()

	targets := goTLSTargetsFromELFSymbols(exe)
	if len(targets) == 0 {
		targets = goTLSTargetsFromPCLN(exe)
	}
	if len(targets) == 0 {
		err := fmt.Errorf("no Go TLS functions found in %s", binPath)
		goTLSTargetCache.Store(cacheKey, goTLSTargetCacheEntry{size: info.Size(), modUnixNano: info.ModTime().UnixNano(), err: err.Error()})
		return nil, err
	}

	goTLSTargetCache.Store(cacheKey, goTLSTargetCacheEntry{
		size:        info.Size(),
		modUnixNano: info.ModTime().UnixNano(),
		targets:     cloneGoTLSTargets(targets),
	})
	return targets, nil
}
