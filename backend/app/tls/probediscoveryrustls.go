package tls

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"sort"
)

// RustlsOffsets stores the rustls plaintext-touching function offsets inside a
// stripped static-pie Rust binary (codex, cursor, etc.).
//
//   - WriteTLS: entry of `RecordLayer::encrypt_outgoing` (rustls
//     record_layer.rs). On entry the SysV-AMD64 ABI places (rdi=sret return
//     slot, rsi=&mut self RecordLayer, rdx=&OutboundPlainMessage). The borrowed
//     OutboundPlainMessage carries the about-to-be-encrypted plaintext slice
//     (Single variant: +0x08=ptr, +0x10=len). The dedicated
//     uprobe_rustls_encrypt_outgoing eBPF program dereferences it.
//   - ReadTLS: entry of the rustls inbound read_tls guard region. Currently
//     located but NOT attached: read_tls reads ENCRYPTED socket bytes (not
//     plaintext), and the only clean plaintext RECV exit (Reader::read) lacks a
//     stable string anchor in stripped binaries. RECV plaintext capture is a
//     follow-up. Capturing ciphertext is rejected (no encrypted TLS capture).
type RustlsOffsets struct {
	WriteTLS uint64
	ReadTLS  uint64
}

// FindRustlsOffsets locates the rustls plaintext functions in a Rust binary.
// It first tries the symbol table (non-stripped builds); if that fails it
// falls back to the eh_frame + rodata-cross-reference scanner, which works on
// stripped static-pie binaries (codex).
func FindRustlsOffsets(binPath string) (*RustlsOffsets, error) {
	exe, err := elf.Open(binPath)
	if err != nil {
		return nil, fmt.Errorf("open binary: %w", err)
	}
	defer exe.Close()

	// 1. Symbol table (non-stripped builds).
	if offsets := trySymbolTable(exe); offsets != nil {
		return offsets, nil
	}

	// 2. eh_frame + rodata cross-reference (stripped static-pie builds).
	offsets, err := findRustlsOffsetsViaEHFrame(binPath, exe)
	if err != nil {
		// 3. Legacy byte-pattern heuristic (last resort, low precision).
		log.Printf("[rustls] eh_frame scan failed for %s: %v; falling back to byte-pattern heuristic", binPath, err)
		return scanTextSection(exe)
	}
	return offsets, nil
}

func trySymbolTable(exe *elf.File) *RustlsOffsets {
	symbols, _ := exe.Symbols()
	if len(symbols) == 0 {
		symbols, _ = exe.DynamicSymbols()
	}

	var offsets RustlsOffsets
	var foundWrite, foundRead bool

	for _, sym := range symbols {
		if containsRustlsWrite(sym.Name) && sym.Value > 0 {
			offsets.WriteTLS = sym.Value
			foundWrite = true
		}
		if containsRustlsRead(sym.Name) && sym.Value > 0 {
			offsets.ReadTLS = sym.Value
			foundRead = true
		}
	}

	if foundWrite && foundRead {
		return &offsets
	}
	return nil
}

func containsRustlsWrite(name string) bool {
	patterns := []string{
		"rustls::connection::Connection::write_tls",
		"rustls::conn::Connection::write_tls",
		"_ZN6rustls10connection10Connection9write_tls",
	}
	for _, p := range patterns {
		if name == p {
			return true
		}
	}
	return false
}

func containsRustlsRead(name string) bool {
	patterns := []string{
		"rustls::connection::Connection::read_tls",
		"rustls::conn::Connection::read_tls",
		"_ZN6rustls10connection10Connection8read_tls",
	}
	for _, p := range patterns {
		if name == p {
			return true
		}
	}
	return false
}

// ---- eh_frame + rodata cross-reference scanner ----
//
// Stripped Rust binaries still ship .eh_frame (used for unwinding/panics),
// which contains one FDE per function with its exact entry vaddr. Stripping
// only removes .symtab, not .eh_frame — so every function entry remains
// discoverable.
//
// To pin rustls *specific* functions among the hundreds of thousands of FDEs,
// we cross-reference stable panic/assert strings embedded in .rodata:
//
//   - "received plaintext buffer full"        -> inside rustls read_tls guard (RECV)
//   - "assertion failed: self.next_pre_encrypt_action() != PreEncryptAction::Refuse"
//                                            -> inside rustls encrypt_outgoing (SEND)
//   - "traffic keys exhausted, closing connection to prevent security failure"
//                                            -> inside the send traffic-key-refresh path (SEND)
//
// Algorithm:
//  1. Locate each anchor string's vaddr in .rodata.
//  2. Scan .text for `lea reg, [rip+disp32]` whose effective target equals
//     that vaddr (the compiler's reference to the string).
//  3. For each such LEA site, find the .eh_frame FDE whose [start,end) covers
//     it; that FDE's start is the enclosing function entry.
//
// This is deterministic, version-independent (the anchor strings are long,
// distinctive, and part of rustls's public behavior), and avoids the
// fragile generic-function-prologue matching of the old byte-pattern scanner.

// rustls anchor strings. Order matters only for classification (send/recv).
//
// SEND anchors pin `encrypt_outgoing` (record_layer.rs) — the function that
// receives a borrowed `OutboundPlainMessage` (with the plaintext slice) and
// encrypts it. The "traffic keys exhausted" string lives in
// `CommonState::write_plaintext` (common_state.rs), NOT encrypt_outgoing, so
// it must NOT be a send anchor (it pins the wrong function).
//
// RECV anchors pin `ConnectionCommon::read_tls` (conn.rs:760) — its guard
// returns "received plaintext buffer full". This string is packed adjacent to
// many other rustls strings in .rodata, so it occurs multiple times; we match
// every occurrence and let the FDE with the most distinct reference sites win.
var rustlsAnchorStrings = []struct {
	kind  string // "send" or "recv"
	value []byte
}{
	{"send", []byte("assertion failed: self.next_pre_encrypt_action() != PreEncryptAction::Refuse")},
	{"recv", []byte("received plaintext buffer full")},
}

func findRustlsOffsetsViaEHFrame(binPath string, exe *elf.File) (*RustlsOffsets, error) {
	textSec := exe.Section(".text")
	if textSec == nil {
		return nil, errors.New(".text section not found")
	}
	rodataSec := exe.Section(".rodata")
	if rodataSec == nil {
		return nil, errors.New(".rodata section not found")
	}
	textData, err := textSec.Data()
	if err != nil {
		return nil, fmt.Errorf("read .text: %w", err)
	}
	textVA := textSec.Addr
	rodataData, err := rodataSec.Data()
	if err != nil {
		return nil, fmt.Errorf("read .rodata: %w", err)
	}
	rodataVA := rodataSec.Addr

	// Parse .eh_frame FDE pc ranges.
	fdes, err := parseEHFrameFDEs(binPath)
	if err != nil {
		return nil, fmt.Errorf("parse .eh_frame: %w", err)
	}
	if len(fdes) == 0 {
		return nil, errors.New("no FDEs found in .eh_frame")
	}
	// Sort by start for binary search.
	sort.Slice(fdes, func(i, j int) bool { return fdes[i].start < fdes[j].start })
	starts := make([]uint64, len(fdes))
	for i, f := range fdes {
		starts[i] = f.start
	}
	enclosing := func(va uint64) (uint64, bool) {
		// Largest start <= va.
		idx := sort.Search(len(starts), func(i int) bool { return starts[i] > va }) - 1
		if idx < 0 {
			return 0, false
		}
		f := fdes[idx]
		if f.start <= va && va < f.end {
			return f.start, true
		}
		return 0, false
	}

	// Collect candidate function entries per direction.
	sendCands := map[uint64]int{}
	recvCands := map[uint64]int{}
	for _, a := range rustlsAnchorStrings {
		// An anchor string may appear multiple times in .rodata (packed
		// adjacent to other strings). Each occurrence is referenced by a
		// different LEA site; match every occurrence so the true rustls
		// function (which references the specific copy) is credited.
		for _, va := range findAllStringVAs(rodataData, rodataVA, a.value) {
			for _, ref := range findRIPRelLEARefs(textData, textVA, va) {
				entry, ok := enclosing(ref)
				if !ok {
					continue
				}
				switch a.kind {
				case "send":
					sendCands[entry]++
				case "recv":
					recvCands[entry]++
				}
			}
		}
	}

	var offsets RustlsOffsets
	if e, ok := bestCandidate(sendCands); ok {
		offsets.WriteTLS = e
	}
	if e, ok := bestRecvCandidate(recvCands, fdes); ok {
		offsets.ReadTLS = e
	}

	if offsets.WriteTLS == 0 && offsets.ReadTLS == 0 {
		return nil, errors.New("no rustls function entries located via eh_frame cross-reference")
	}
	log.Printf("[rustls] eh_frame scan: send=0x%x recv=0x%x (send-cands=%v recv-cands=%v)",
		offsets.WriteTLS, offsets.ReadTLS, keysOf(sendCands), keysOf(recvCands))
	return &offsets, nil
}

// fdeRange is a single .eh_frame FDE pc range [start, end).
type fdeRange struct{ start, end uint64 }

// parseEHFrameFDEs extracts all FDE pc ranges from .eh_frame. It parses the
// CIE/FDE structure directly rather than shelling out to objdump, so it works
// without external tooling at runtime.
func parseEHFrameFDEs(binPath string) ([]fdeRange, error) {
	exe, err := elf.Open(binPath)
	if err != nil {
		return nil, err
	}
	defer exe.Close()

	sec := exe.Section(".eh_frame")
	if sec == nil {
		return nil, errors.New(".eh_frame section not found")
	}
	data, err := sec.Data()
	if err != nil {
		return nil, err
	}
	ehVA := sec.Addr

	var out []fdeRange
	// .eh_frame is a sequence of length-prefixed CIE/FDE entries.
	// We only need FDEs (which carry the pc_begin/pc_range). Each entry begins
	// with a 4-byte length (or 12-byte extended length if length==0xffffffff).
	// The first 4 bytes after length are the CIE pointer; for an FDE this is a
	// non-zero 32-bit offset (relative to the current entry), for a CIE it is 0.
	is64bit := exe.Class == elf.ELFCLASS64
	var endian binary.ByteOrder = binary.LittleEndian
	if exe.Data == elf.ELFDATA2MSB {
		endian = binary.BigEndian
	}

	pos := 0
	for pos+4 <= len(data) {
		length := endian.Uint32(data[pos:])
		entryStart := pos + 4
		var entryLen int
		if length == 0 {
			break // terminator
		}
		if length == 0xffffffff {
			// extended length (64-bit)
			if entryStart+8 > len(data) {
				break
			}
			entryLen = int(endian.Uint64(data[entryStart:]))
			entryStart += 8
		} else {
			entryLen = int(length)
		}
		entryEnd := entryStart + entryLen
		if entryEnd > len(data) {
			break
		}
		body := data[entryStart:entryEnd]

		// CIE pointer (4 bytes for 32-bit DWARF). If zero -> this is a CIE.
		if len(body) < 4 {
			pos = entryEnd
			continue
		}
		ciePtr := endian.Uint32(body)
		if ciePtr == 0 {
			// CIE — skip (but we rely on its address_size; assume 8 for x86_64).
			pos = entryEnd
			continue
		}
		// FDE. Parse PC begin and range using the CIE's augmentation encoding.
		// We need the CIE to know the FDE encoding (often DW_EH_PE_pcrel|sdata4
		// for pc_begin and DW_EH_PE_udata4 for range, the GCC/Linux default).
		// fdeVA points at the start of this FDE's length word.
		fdeVA := ehVA + uint64(pos)
		cieOff := uint64(ciePtr) // offset from the FDE's CIE pointer field back to its CIE
		_ = cieOff
		// Locate CIE body to read augmentation string and FDE encoding.
		cieBody, fdeEnc, ok := readCIEForFDE(data, ehVA, fdeVA, uint64(ciePtr), endian, is64bit)
		if !ok {
			pos = entryEnd
			continue
		}
		_ = cieBody
		// FDE body layout after the CIE pointer:
		//   pc_begin (per fdeEnc), pc_range (per rangeEnc from CIE aug 'R'),
		//   aug data, then CFI instructions.
		// Default GCC encoding: pc_begin = DW_EH_PE_pcrel|sdata4, range = udata4.
		// fdeBodyVA is the vaddr of the pc_begin field itself (the pcrel base).
		fdeBodyVA := fdeVA + 8
		pcBegin, pcRange, consumed, ok := parseFDEAddrs(body[4:], fdeBodyVA, fdeEnc, endian)
		if !ok {
			pos = entryEnd
			continue
		}
		_ = consumed
		if pcRange > 0 {
			out = append(out, fdeRange{start: pcBegin, end: pcBegin + pcRange})
		}
		pos = entryEnd
	}
	return out, nil
}

// readCIEForFDE locates and parses the CIE associated with an FDE to extract
// the FDE address-encoding byte (augmentation 'R'). Returns the CIE body,
// the FDE encoding, and ok.
func readCIEForFDE(data []byte, ehVA, fdeVA, ciePtr uint64, endian binary.ByteOrder, is64bit bool) ([]byte, byte, bool) {
	// ciePtr is the byte offset from the location of the CIE pointer field
	// itself back to the associated CIE (start of its length field).
	ciePtrFieldVA := fdeVA + 4 // CIE pointer field sits right after the FDE length word
	cieLenFieldVA := ciePtrFieldVA - ciePtr
	cieLenFieldOff := int(cieLenFieldVA - ehVA)
	if cieLenFieldOff < 0 || cieLenFieldOff+4 > len(data) {
		return nil, 0, false
	}
	cieLen := endian.Uint32(data[cieLenFieldOff:])
	cieBodyStart := cieLenFieldOff + 4
	var cieBodyLen int
	if cieLen == 0xffffffff {
		if cieBodyStart+8 > len(data) {
			return nil, 0, false
		}
		cieBodyLen = int(endian.Uint64(data[cieBodyStart:]))
		cieBodyStart += 8
	} else {
		cieBodyLen = int(cieLen)
	}
	if cieBodyStart+cieBodyLen > len(data) || cieBodyLen < 4 {
		return nil, 0, false
	}
	cie := data[cieBodyStart : cieBodyStart+cieBodyLen]
	// CIE body: version (1 byte), aug string (null-terminated), then
	// (for v1+) code_alignment_factor (uleb), data_alignment_factor (sleb),
	// return_address_register (uleb for v3, 1 byte for v1), then aug data.
	p := 0
	if len(cie) < 5 {
		return nil, 0, false
	}
	version := cie[p]
	p++
	// augmentation string
	augStart := p
	for p < len(cie) && cie[p] != 0 {
		p++
	}
	aug := string(cie[augStart:p])
	p++ // skip null
	if version >= 4 {
		p += 2 // address_size, segment_size
	}
	// uleb: code_align
	_, n := readULEB(cie[p:])
	p += n
	// sleb: data_align
	_, n = readSLEB(cie[p:])
	p += n
	// return addr reg
	if version >= 3 {
		_, n = readULEB(cie[p:])
		p += n
	} else {
		p++ // 1 byte
	}
	// augmentation data: only if aug starts with 'z'
	fdeEnc := byte(0x1B) // default: pcrel | sdata4 (the common Linux/GCC default)
	if len(aug) > 0 && aug[0] == 'z' {
		augLen, n := readULEB(cie[p:])
		p += n
		augData := cie[p : p+int(augLen)]
		ap := 0
		for i := 1; i < len(aug); i++ {
			switch aug[i] {
			case 'R':
				if ap < len(augData) {
					fdeEnc = augData[ap]
					ap++
				}
			case 'L', 'B', 'P':
				ap++ // one byte each
			case 'S':
				// no data
			}
		}
	}
	return cie, fdeEnc, true
}

// parseFDEAddrs decodes pc_begin (per enc) and pc_range (udata4 default) from
// the FDE body after the CIE pointer. fdeBodyVA is the vaddr of the first byte
// after the CIE pointer field (used for pcrel resolution).
func parseFDEAddrs(body []byte, fdeBodyVA uint64, enc byte, endian binary.ByteOrder) (pcBegin, pcRange uint64, consumed int, ok bool) {
	// Decode pc_begin according to enc. We handle the common formats used by
	// GCC/clang on Linux x86_64/aarch64: pcrel|sdata4 (0x1B), absptr (0x00),
	// textrel|sdata4, udata4.
	pcBegin, n, ok := decodeEHHdr(body, fdeBodyVA, enc, endian)
	if !ok {
		return 0, 0, 0, false
	}
	body = body[n:]
	// pc_range is always an unsigned value in the same size class as the
	// pc_begin encoding's size (per .eh_frame spec). Use the size implied by enc.
	size := ehEncSize(enc)
	if size == 0 {
		size = 4
	}
	if len(body) < size {
		return 0, 0, 0, false
	}
	switch size {
	case 4:
		pcRange = uint64(endian.Uint32(body))
	case 8:
		pcRange = endian.Uint64(body)
	case 2:
		pcRange = uint64(endian.Uint16(body))
	default:
		return 0, 0, 0, false
	}
	return pcBegin, pcRange, n + size, true
}

// decodeEHHdr decodes one DW_EH_PE value from data at vaddr fieldVA.
func decodeEHHdr(data []byte, fieldVA uint64, enc byte, endian binary.ByteOrder) (val uint64, n int, ok bool) {
	if enc == 0xFF { // DW_EH_PE_omit
		return 0, 0, false
	}
	size := ehEncSize(enc)
	if size == 0 || len(data) < size {
		return 0, 0, false
	}
	var raw uint64
	switch size {
	case 4:
		raw = uint64(endian.Uint32(data))
	case 8:
		raw = endian.Uint64(data)
	case 2:
		raw = uint64(endian.Uint16(data))
	default:
		return 0, 0, false
	}
	n = size
	// Sign-extend signed data formats BEFORE applying the base, so pcrel math is correct.
	fmt2 := enc & 0x0F
	switch fmt2 {
	case 0x0A: // sdata2
		if raw&0x8000 != 0 {
			raw |= 0xFFFFFFFFFFFF0000
		}
	case 0x0B: // sdata4
		if raw&0x80000000 != 0 {
			raw |= 0xFFFFFFFF00000000
		}
	}
	// Apply modifier (high 4 bits = base).
	mod := enc >> 4
	switch mod {
	case 0x0: // absptr
		val = raw
	case 0x1: // pcrel
		// pcrel is relative to the address of the field itself.
		val = fieldVA + raw
	case 0x2: // textrel
		val = raw // approximate (text base unknown here); rare
	case 0x3: // datarel
		val = raw
	default:
		val = raw
	}
	return val, n, true
}

func ehEncSize(enc byte) int {
	// DW_EH_PE format is in the low nibble; size class in bytes (0 = variable/omit).
	//   0x00 absptr  -> pointer size (8 on 64-bit)
	//   0x01 uleb128 -> variable (caller must handle; we treat as unknown here)
	//   0x02 udata2  -> 2
	//   0x03 udata4  -> 4
	//   0x04 udata8  -> 8
	//   0x09 sleb128 -> variable
	//   0x0A sdata2  -> 2
	//   0x0B sdata4  -> 4
	//   0x0C sdata8  -> 8
	switch enc & 0x0F {
	case 0x00: // absptr — pointer size
		return 8
	case 0x02: // udata2
		return 2
	case 0x03: // udata4
		return 4
	case 0x04: // udata8
		return 8
	case 0x0A: // sdata2
		return 2
	case 0x0B: // sdata4
		return 4
	case 0x0C: // sdata8
		return 8
	}
	return 0
}

func readULEB(b []byte) (uint64, int) {
	var val uint64
	var shift uint
	for i := 0; i < len(b); i++ {
		c := b[i]
		val |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			return val, i + 1
		}
		shift += 7
	}
	return val, len(b)
}

func readSLEB(b []byte) (int64, int) {
	var val int64
	var shift uint
	for i := 0; i < len(b); i++ {
		c := b[i]
		val |= int64(c&0x7f) << shift
		shift += 7
		if c&0x80 == 0 {
			if c&0x40 != 0 {
				val |= -1 << shift
			}
			return val, i + 1
		}
	}
	return val, len(b)
}

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

// hasRustlsStrings checks if the binary contains rustls-related string constants,
// which indicates it's a Rust binary with rustls compiled in (even if stripped).
func hasRustlsStrings(binPath string) bool {
	exe, err := elf.Open(binPath)
	if err != nil {
		return false
	}
	defer exe.Close()

	rodata := exe.Section(".rodata")
	if rodata == nil {
		return false
	}

	data, err := rodata.Data()
	if err != nil {
		return false
	}

	patterns := [][]byte{
		[]byte("rustls"),
		[]byte("src/conn.rs"),
		[]byte("tokio-rustls"),
		[]byte("Connection::write_tls"),
		[]byte("Connection::read_tls"),
	}

	for _, pattern := range patterns {
		if bytes.Contains(data, pattern) {
			return true
		}
	}
	return false
}
