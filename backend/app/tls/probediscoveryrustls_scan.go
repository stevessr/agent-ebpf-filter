package tls

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"sort"
)

// findStringVA returns the vaddr of the first occurrence of s in rodata, or
// (0,false) if not found. rodataVA is the section's virtual address.
func findStringVA(rodataData []byte, rodataVA uint64, s []byte) (uint64, bool) {
	i := bytes.Index(rodataData, s)
	if i < 0 {
		return 0, false
	}
	return rodataVA + uint64(i), true
}

// findAllStringVAs returns the vaddr of every occurrence of `s` in .rodata.
// rustls packs panic/assert strings adjacently, so a short anchor like
// "received plaintext buffer full" can appear many times (each followed by a
// different neighbor string). Each occurrence has its own LEA reference site,
// so we must consider all of them to credit the correct enclosing function.
func findAllStringVAs(rodataData []byte, rodataVA uint64, s []byte) []uint64 {
	var out []uint64
	off := 0
	for {
		i := bytes.Index(rodataData[off:], s)
		if i < 0 {
			break
		}
		out = append(out, rodataVA+uint64(off+i))
		off += i + len(s)
	}
	return out
}

// findRIPRelLEARefs scans .text for `lea reg, [rip+disp32]` instructions whose
// effective target equals targetVA, returning the vaddrs of each such LEA.
//
// Encoding (x86_64): [REX?] 0x8d <modrm: mod=00, rm=101> <disp32>
//   - REX is optional (0x40..0x4f); present when using r8-r15 or 64-bit ops.
//   - modrm low 3 bits (rm) == 101 and high 2 bits (mod) == 00 => RIP-relative.
//
// Effective target = (next_instr_vaddr) + sign-extended disp32.
func findRIPRelLEARefs(textData []byte, textVA, targetVA uint64) []uint64 {
	var refs []uint64
	n := len(textData)
	for i := 0; i < n-7; i++ {
		b := textData[i]
		j := i
		if b >= 0x40 && b <= 0x4f {
			j = i + 1
			if j >= n || textData[j] != 0x8d {
				continue
			}
		} else if b != 0x8d {
			continue
		}
		if textData[j] != 0x8d {
			continue
		}
		if j+6 > n {
			continue
		}
		modrm := textData[j+1]
		if (modrm>>6)&3 != 0 || modrm&7 != 5 {
			continue
		}
		disp := int64(int32(binary.LittleEndian.Uint32(textData[j+2:])))
		nextInstrVA := textVA + uint64(j) + 6
		if nextInstrVA+uint64(disp) == targetVA {
			refs = append(refs, textVA+uint64(i))
		}
	}
	return refs
}

// bestCandidate returns the function entry that scored highest (most anchor
// hits), preferring the smallest-address entry on ties for determinism.
func bestCandidate(cands map[uint64]int) (uint64, bool) {
	if len(cands) == 0 {
		return 0, false
	}
	var best uint64
	bestScore := -1
	// iterate in sorted order for determinism
	keys := make([]uint64, 0, len(cands))
	for k := range cands {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		if cands[k] > bestScore {
			bestScore = cands[k]
			best = k
		}
	}
	return best, true
}

// bestRecvCandidate selects the RECV function entry (rustls `ConnectionCommon::
// read_tls`) from the set of candidates that reference the "received plaintext
// buffer full" anchor string.
//
// The challenge: that anchor string is packed adjacently with other rustls
// strings in .rodata, so it appears many times. Each occurrence is referenced
// by a different LEA site, and rustls's generic `ConnectionCommon<Data>` is
// monomorphized for several `Data` types (ClientConnection, ServerConnection,
// plus hyper/tokio-rustls wrappers), producing many copies of any function
// that touches the string. The result is a swarm of false-positive candidates
// that are monomorphized copies of state-machine functions (process_new_packets
// and friends), NOT read_tls.
//
// read_tls is a small leaf-ish function that reads from the socket; the false
// positives are large state machines. More importantly, monomorphization
// produces functions of IDENTICAL size (one generic instantiation yields
// several same-sized copies), while read_tls is a single non-generic function
// whose size is unique among the candidates.
//
// Heuristic:
//  1. Compute each candidate's FDE size.
//  2. Group candidates by size. Drop sizes that occur more than once — those
//     are monomorphization groups (identical-size copies = same source fn
//     instantiated for multiple type params).
//  3. Among the remaining (unique-size) candidates, pick the smallest. read_tls
//     is the smallest unique function; the other unique candidate (if any) is
//     a large state machine.
//
// On codex 0.142 this yields read_tls@0x7be3b30 (size 0x262, unique) cleanly,
// rejecting the 14× size-0x426 monomorphization group and the 5× size-0x22f3
// group.
func bestRecvCandidate(cands map[uint64]int, fdes []fdeRange) (uint64, bool) {
	if len(cands) == 0 {
		return 0, false
	}
	// sizeOf returns the FDE length covering entry, or 0 if unknown.
	sizeOf := func(entry uint64) uint64 {
		for _, f := range fdes {
			if f.start == entry {
				return f.end - f.start
			}
		}
		return 0
	}
	// Count size frequency among candidates.
	sizeFreq := map[uint64]int{}
	for entry := range cands {
		sizeFreq[sizeOf(entry)]++
	}
	// Collect candidates whose size is unique (freq == 1) and non-zero.
	type cand struct {
		entry uint64
		size  uint64
	}
	var unique []cand
	for entry := range cands {
		sz := sizeOf(entry)
		if sz > 0 && sizeFreq[sz] == 1 {
			unique = append(unique, cand{entry, sz})
		}
	}
	if len(unique) == 0 {
		// No unique-size candidate — fall back to plain bestCandidate.
		return bestCandidate(cands)
	}
	// Pick the smallest unique-size candidate (read_tls is small; the only
	// other unique candidate, if present, is a large state machine).
	sort.Slice(unique, func(i, j int) bool { return unique[i].size < unique[j].size })
	return unique[0].entry, true
}

func keysOf(m map[uint64]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, fmt.Sprintf("0x%x", k))
	}
	return out
}

// ---- legacy byte-pattern heuristic (fallback) ----

func scanTextSection(exe *elf.File) (*RustlsOffsets, error) {
	textSection := exe.Section(".text")
	if textSection == nil {
		return nil, fmt.Errorf(".text section not found")
	}

	data, err := textSection.Data()
	if err != nil {
		return nil, fmt.Errorf("read .text: %w", err)
	}

	writeOffset := findRustlsWritePattern(data, textSection.Addr)
	readOffset := findRustlsReadPattern(data, textSection.Addr)

	if writeOffset == 0 && readOffset == 0 {
		return nil, fmt.Errorf("rustls functions not found")
	}

	return &RustlsOffsets{
		WriteTLS: writeOffset,
		ReadTLS:  readOffset,
	}, nil
}

// findRustlsWritePattern 查找 write_tls 的字节码模式
func findRustlsWritePattern(data []byte, baseAddr uint64) uint64 {
	patterns := [][]byte{
		{0x55, 0x48, 0x89, 0xe5},
		{0x48, 0x83, 0xec},
	}

	for i := 0; i < len(data)-100; i++ {
		if matchesPattern(data[i:], patterns[0]) {
			if i+10 < len(data) && matchesPattern(data[i+4:], patterns[1][:2]) {
				if hasWriteSyscallNearby(data, i) {
					return baseAddr + uint64(i)
				}
			}
		}
	}
	return 0
}

func findRustlsReadPattern(data []byte, baseAddr uint64) uint64 {
	patterns := [][]byte{
		{0x55, 0x48, 0x89, 0xe5},
		{0x48, 0x83, 0xec},
	}

	for i := 0; i < len(data)-100; i++ {
		if matchesPattern(data[i:], patterns[0]) {
			if i+10 < len(data) && matchesPattern(data[i+4:], patterns[1][:2]) {
				if hasReadSyscallNearby(data, i) {
					return baseAddr + uint64(i)
				}
			}
		}
	}
	return 0
}

func matchesPattern(data []byte, pattern []byte) bool {
	if len(data) < len(pattern) {
		return false
	}
	for i := range pattern {
		if data[i] != pattern[i] {
			return false
		}
	}
	return true
}

func hasWriteSyscallNearby(data []byte, offset int) bool {
	end := offset + 200
	if end > len(data) {
		end = len(data)
	}

	for i := offset; i < end-8; i++ {
		if data[i] == 0x0f && data[i+1] == 0x05 {
			if hasSyscallNumber(data[offset:i], 1) || hasSyscallNumber(data[offset:i], 20) {
				return true
			}
		}
	}
	return false
}

func hasReadSyscallNearby(data []byte, offset int) bool {
	end := offset + 200
	if end > len(data) {
		end = len(data)
	}

	for i := offset; i < end-8; i++ {
		if data[i] == 0x0f && data[i+1] == 0x05 {
			if hasSyscallNumber(data[offset:i], 0) || hasSyscallNumber(data[offset:i], 19) {
				return true
			}
		}
	}
	return false
}

func hasSyscallNumber(data []byte, num uint32) bool {
	for i := 0; i < len(data)-5; i++ {
		if data[i] == 0xb8 {
			val := binary.LittleEndian.Uint32(data[i+1:])
			if val == num {
				return true
			}
		}
		if i < len(data)-7 && data[i] == 0x48 && data[i+1] == 0xc7 && data[i+2] == 0xc0 {
			val := binary.LittleEndian.Uint32(data[i+3:])
			if val == num {
				return true
			}
		}
	}
	return false
}
