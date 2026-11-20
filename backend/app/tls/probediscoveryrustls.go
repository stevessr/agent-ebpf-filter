package tls

import (
	"bytes"
	"debug/elf"
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
//   - ReadTLS: entry of the rustls RECV plaintext-consume function. This is the
//     inline-combined `Reader::consume` + `ChunkVecBuffer::consume_first_chunk`
//     (rustls conn.rs:297 + vecbuf.rs:159), located via the
//     "illegal `BufRead::consume` usage" release-assert string. On entry
//     (rdi=&Reader, rsi=amt): r14 = *rdi = &ChunkVecBuffer (received_plaintext),
//     and rsi = amt = the number of plaintext bytes the application is consuming
//     from the front chunk. The dedicated uprobe_rustls_consume_first_chunk eBPF
//     program navigates the ChunkVecBuffer + VecDeque layout to emit exactly
//     min(amt, available) plaintext bytes — no duplicates, no gaps. This is the
//     clean RECV plaintext exit; read_tls (which reads ENCRYPTED socket bytes)
//     is intentionally not used. Capturing ciphertext is rejected by constraint.
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
//   - "assertion failed: self.next_pre_encrypt_action() != PreEncryptAction::Refuse"
//                                            -> inside rustls encrypt_outgoing (SEND)
//   - "illegal `BufRead::consume` usage"    -> inside ChunkVecBuffer::consume_first_chunk
//                                            (RECV plaintext-consume path)
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
// RECV anchors pin the inline-combined `Reader::consume` +
// `ChunkVecBuffer::consume_first_chunk` (conn.rs:297 + vecbuf.rs:159) via the
// "illegal `BufRead::consume` usage" release assert. This function receives
// (&Reader, amt) where *Reader = &ChunkVecBuffer (received_plaintext) and amt
// is the plaintext byte count being consumed — the clean RECV plaintext exit.
// The string is long, distinctive, and appears exactly once in .rodata, so it
// pins a single function with no monomorphization ambiguity (unlike the old
// "received plaintext buffer full" anchor, which appeared 13+ times and
// required fragile size-frequency filtering).
var rustlsAnchorStrings = []struct {
	kind  string // "send" or "recv"
	value []byte
}{
	{"send", []byte("assertion failed: self.next_pre_encrypt_action() != PreEncryptAction::Refuse")},
	{"recv", []byte("illegal `BufRead::consume` usage")},
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
	// RECV anchor ("illegal `BufRead::consume` usage") is unique in .rodata and
	// pins a single function, so plain bestCandidate suffices. The old
	// "received plaintext buffer full" anchor needed bestRecvCandidate's
	// size-frequency filtering because it appeared 13+ times; that path is kept
	// only as a fallback if the unique anchor yields no candidate.
	if e, ok := bestCandidate(recvCands); ok {
		offsets.ReadTLS = e
	} else if e, ok := bestRecvCandidate(recvCands, fdes); ok {
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
