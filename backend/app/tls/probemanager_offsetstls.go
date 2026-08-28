package tls

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cilium/ebpf/link"
)

type sslBytePattern struct {
	name    string
	pattern []byte
	mask    []byte // 0x00 = wildcard, 0xff = exact match
}

var (
	// BoringSSL patterns (Node.js, Bun, Claude CLI)
	bsSSLRead = sslBytePattern{
		name: "BoringSSL-SSL_read",
		pattern: []byte{
			0x55, 0x48, 0x89, 0xe5, 0x41, 0x57, 0x41, 0x56,
			0x53, 0x50, 0x48, 0x83, 0xbf, 0x98, 0x00, 0x00,
			0x00, 0x00, 0x74,
		},
	}
	bsSSLWrite = sslBytePattern{
		name: "BoringSSL-SSL_write",
		pattern: []byte{
			0x55, 0x48, 0x89, 0xe5, 0x41, 0x57, 0x41, 0x56,
			0x41, 0x55, 0x41, 0x54, 0x53, 0x48, 0x83, 0xec,
			0x18, 0x41, 0x89, 0xd7, 0x49, 0x89, 0xf6, 0x48,
			0x89, 0xfb,
		},
	}

	// OpenSSL 3.x generic function prologue + SSL_write/SSL_read common prefix.
	// Uses wildcard bytes (mask) to match both OpenSSL and AWS-LC variants.
	// Pattern: push rbp; mov rbp,rsp; push r15; push r14; push r13; push r12; push rbx; sub rsp, XX
	osslSSLCommonPrefix = sslBytePattern{
		name: "OpenSSL-common-prefix",
		pattern: []byte{
			0x55, 0x48, 0x89, 0xe5, 0x41, 0x57, 0x41, 0x56,
			0x41, 0x55, 0x41, 0x54, 0x53, 0x48, 0x83, 0xec,
			0x00, // ← wildcard: sub rsp, XX (varies)
		},
		mask: []byte{
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			0x00,
		},
	}
)

// buildMask creates a mask from a pattern where 0x00 means wildcard.
func buildMask(pat []byte) []byte {
	m := make([]byte, len(pat))
	for i, b := range pat {
		if b == 0x00 {
			m[i] = 0x00
		} else {
			m[i] = 0xff
		}
	}
	return m
}

// matchMasked reports whether data[pos:] matches pat under mask.
// mask[i]==0x00 means wildcard (match any byte).
func matchMasked(data, pat, mask []byte) bool {
	for i, b := range pat {
		if mask != nil && mask[i] == 0x00 {
			continue // wildcard
		}
		if data[i] != b {
			return false
		}
	}
	return true
}

// findMasked finds the first match of pat under mask, returns offset or -1.
func findMasked(data, pat, mask []byte) int64 {
	if len(pat) > len(data) {
		return -1
	}
	if mask == nil {
		mask = buildMask(pat)
	}
	for i := 0; i <= len(data)-len(pat); i++ {
		if matchMasked(data[i:], pat, mask) {
			return int64(i)
		}
	}
	return -1
}

// binaryEmbedsSSL checks if the binary contains the string "SSL_write",
// indicating it has statically-linked OpenSSL/BoringSSL/AWS-LC.
func binaryEmbedsSSL(binPath string) bool {
	data, err := os.ReadFile(binPath)
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte("SSL_write"))
}

// AttachBoringSSLByOffsets searches for BoringSSL or OpenSSL function prologue
// byte patterns in a stripped binary and attaches uprobes by file offset.
func (m *TLSProbeManager) AttachBoringSSLByOffsets(binPath string, pid int) error {
	if m == nil {
		return fmt.Errorf("manager is nil")
	}
	if !binaryEmbedsSSL(binPath) {
		return fmt.Errorf("binary does not embed SSL (no 'SSL_write' string found)")
	}
	log.Printf("[tls] binary embeds SSL, searching for function patterns...")

	data, err := os.ReadFile(binPath)
	if err != nil {
		return fmt.Errorf("read binary: %w", err)
	}

	var readOff, writeOff int64
	found := false

	// Strategy 1: BoringSSL exact patterns + relative offset validation
	readOff = findBS(data, bsSSLRead.pattern)
	if readOff >= 0 {
		writeCenter := readOff + 0xCA0
		writeOff = findBSNear(data, bsSSLWrite.pattern, writeCenter, 0x10000)
		if writeOff >= 0 {
			log.Printf("[tls] → matched BoringSSL patterns: SSL_read=%#x SSL_write=%#x", readOff, writeOff)
			found = true
		}
	}

	// Strategy 2: OpenSSL common prefix pattern (with wildcard)
	if !found {
		osslMask := buildMask(osslSSLCommonPrefix.pattern)
		// Find all occurrences of the common prefix
		var matches []int64
		for i := int64(0); i <= int64(len(data))-int64(len(osslSSLCommonPrefix.pattern)); i++ {
			if matchMasked(data[i:], osslSSLCommonPrefix.pattern, osslMask) {
				matches = append(matches, i)
			}
		}
		log.Printf("[tls] OpenSSL common prefix found at %d locations in .text", len(matches))

		// For each match, check if it could be SSL_read or SSL_write by
		// examining nearby bytes for distinguishing features.
		// SSL_read: after common prefix, typically has test/jne on SSL struct field
		//   ... sub rsp, XX; mov REG, rdi; mov QWORD PTR [rbp-0x??], rdi; ...
		// SSL_write: after common prefix, typically saves more args
		// Strategy: take the first N matches as write candidates, last N as read
		if len(matches) >= 2 {
			// Heuristic: SSL_write is usually at a lower address than SSL_read
			// Take the two matches that are closest together in .text
			bestDist := int64(^uint64(0) >> 1)
			for i := 0; i < len(matches); i++ {
				for j := i + 1; j < len(matches); j++ {
					dist := matches[j] - matches[i]
					if dist > 0 && dist < bestDist && dist < 0x10000 {
						bestDist = dist
						writeOff = matches[i]
						readOff = matches[j]
						found = true
					}
				}
			}
			if found {
				log.Printf("[tls] → matched OpenSSL patterns: SSL_write=%#x SSL_read=%#x (dist=%#x)", writeOff, readOff, bestDist)
			}
		}
	}

	if !found {
		return fmt.Errorf("no SSL function patterns matched in stripped binary (%d bytes)", len(data))
	}

	// Attach uprobes by absolute file offset. With cilium/ebpf v0.21 an empty
	// symbol requires UprobeOptions.Address; Offset is only relative to a
	// resolved symbol/Address and therefore cannot be used by itself.
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.objs == nil {
		return fmt.Errorf("TLS probe manager is closed")
	}

	opts := &link.UprobeOptions{}
	if pid > 0 {
		opts.PID = pid
	}

	bin, err := link.OpenExecutable(binPath)
	if err != nil {
		return fmt.Errorf("open executable: %w", err)
	}

	startLinks := len(m.links)
	var errs []error

	if err := attachOffsetProbe(bin, m, "uprobe_ssl_write", uint64(writeOff), false, opts); err != nil {
		errs = append(errs, fmt.Errorf("SSL_write entry: %w", err))
	}
	if err := attachOffsetProbe(bin, m, "uretprobe_ssl_write", uint64(writeOff), true, opts); err != nil {
		errs = append(errs, fmt.Errorf("SSL_write ret: %w", err))
	}
	if err := attachOffsetProbe(bin, m, "uprobe_ssl_read", uint64(readOff), false, opts); err != nil {
		errs = append(errs, fmt.Errorf("SSL_read entry: %w", err))
	}
	if err := attachOffsetProbe(bin, m, "uretprobe_ssl_read", uint64(readOff), true, opts); err != nil {
		errs = append(errs, fmt.Errorf("SSL_read ret: %w", err))
	}

	if len(errs) > 0 {
		for _, l := range m.links[startLinks:] {
			if l != nil {
				_ = l.Close()
			}
		}
		m.links = m.links[:startLinks]
		return fmt.Errorf("offset attach: %w", errors.Join(errs...))
	}
	return nil
}

// findBS searches for byte pattern in data, returns offset or -1.
func findBS(data, pattern []byte) int64 {
	if len(pattern) > len(data) {
		return -1
	}
	for i := 0; i <= len(data)-len(pattern); i++ {
		if bytes.Equal(data[i:i+len(pattern)], pattern) {
			return int64(i)
		}
	}
	return -1
}

// findBSNear searches within ±range of center, returns offset or -1.
func findBSNear(data, pattern []byte, center int64, searchRange int64) int64 {
	start := center - searchRange
	if start < 0 {
		start = 0
	}
	end := center + searchRange
	if end > int64(len(data)) {
		end = int64(len(data))
	}
	off := findBS(data[start:end], pattern)
	if off < 0 {
		return -1
	}
	return start + off
}

// attachOffsetProbe attaches a uprobe/uretprobe at an absolute file offset
// without symbol lookup.
func attachOffsetProbe(bin *link.Executable, m *TLSProbeManager, progName string, address uint64, retprobe bool, baseOpts *link.UprobeOptions) error {
	prog, ok := programByName(&m.objs.AgentTlsCapturePrograms, progName)
	if !ok || prog == nil {
		return fmt.Errorf("TLS BPF program %s is not loaded", progName)
	}
	opts := *baseOpts
	opts.Address = address
	opts.Offset = 0
	var l link.Link
	var err error
	if retprobe {
		l, err = bin.Uretprobe("", prog, &opts)
	} else {
		l, err = bin.Uprobe("", prog, &opts)
	}
	if err != nil {
		return err
	}
	m.links = append(m.links, l)
	return nil
}

// dumpCandidateTLSSymbols opens the binary ELF and logs any symbols that might
// be TLS-related. For Rust binaries (rustls), also dumps all exported symbols
// since they may use mangled names.
func dumpCandidateTLSSymbols(binPath string) {
	f, err := elf.Open(binPath)
	if err != nil {
		log.Printf("[tls]   cannot open ELF for symbol dump: %v", err)
		return
	}
	defer f.Close()
	syms, err := f.Symbols()
	if err != nil {
		syms, _ = f.DynamicSymbols()
	}
	if len(syms) == 0 {
		log.Printf("[tls]   binary is fully stripped — 0 symbols in symtab")
		// Try dynamic symbols as last resort
		dynSyms, _ := f.DynamicSymbols()
		if len(dynSyms) > 0 {
			log.Printf("[tls]   dynamic symbols (%d):", len(dynSyms))
			for _, s := range dynSyms {
				log.Printf("[tls]     %s", s.Name)
			}
		}
		return
	}

	// Look for TLS-related symbols with broader patterns
	candidates := []string{}
	rustlsCandidates := []string{}
	httpCandidates := []string{}

	tlsKeywords := []string{"ssl", "tls", "crypto", "encrypt", "decrypt"}
	rustlsKeywords := []string{"rustls", "webpki", "ClientHello", "ServerHello", "TlsStream"}
	httpKeywords := []string{"hyper", "reqwest", "h2", "http", "HttpRequest", "RequestBuilder"}

	for _, s := range syms {
		name := strings.ToLower(s.Name)
		for _, kw := range tlsKeywords {
			if strings.Contains(name, kw) {
				candidates = append(candidates, s.Name)
				break
			}
		}
		for _, kw := range rustlsKeywords {
			if strings.Contains(name, kw) {
				rustlsCandidates = append(rustlsCandidates, s.Name)
				break
			}
		}
		for _, kw := range httpKeywords {
			if strings.Contains(name, kw) {
				httpCandidates = append(httpCandidates, s.Name)
				break
			}
		}
	}

	if len(candidates) > 0 {
		log.Printf("[tls]   TLS symbols (%d):", len(candidates))
		for _, name := range candidates {
			log.Printf("[tls]     %s", name)
		}
	}
	if len(rustlsCandidates) > 0 {
		log.Printf("[tls]   rustls/webpki symbols (%d):", len(rustlsCandidates))
		for _, name := range rustlsCandidates {
			log.Printf("[tls]     %s", name)
		}
	}
	if len(httpCandidates) > 0 {
		log.Printf("[tls]   HTTP client symbols (%d):", len(httpCandidates))
		for _, name := range httpCandidates {
			log.Printf("[tls]     %s", name)
		}
	}
	if len(candidates)+len(rustlsCandidates)+len(httpCandidates) == 0 {
		log.Printf("[tls]   no TLS/HTTP symbols in symtab (%d total symbols — stripped debuginfo, try nm/objdump)", len(syms))
	}
}
