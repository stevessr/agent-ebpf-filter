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
	osslSSLCommonPrefix = sslBytePattern{
		name: "OpenSSL-common-prefix",
		pattern: []byte{
			0x55, 0x48, 0x89, 0xe5, 0x41, 0x57, 0x41, 0x56,
			0x41, 0x55, 0x41, 0x54, 0x53, 0x48, 0x83, 0xec,
			0x00,
		},
		mask: []byte{
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			0x00,
		},
	}
)

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

func matchMasked(data, pat, mask []byte) bool {
	for i, b := range pat {
		if mask != nil && mask[i] == 0x00 {
			continue
		}
		if data[i] != b {
			return false
		}
	}
	return true
}

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

func binaryEmbedsSSL(binPath string) bool {
	data, err := os.ReadFile(binPath)
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte("SSL_write"))
}

func binaryContainsSSLReadWriteStrings(data []byte) bool {
	return bytes.Contains(data, []byte("SSL_write")) && bytes.Contains(data, []byte("SSL_read"))
}

// selectUnambiguousOpenSSLPair intentionally fails closed. The common prefix
// is not an SSL signature; it is a compiler-generated function prologue that
// can appear throughout a large statically linked binary. Treating the closest
// two out of dozens of hits as SSL_write/SSL_read produces a particularly bad
// failure mode: attach succeeds but captures arbitrary userspace buffers.
//
// We only retain this fallback when the scan produces exactly two candidates,
// both SSL function names are present elsewhere in the binary, and the pair is
// plausibly close without being effectively the same function.
func selectUnambiguousOpenSSLPair(matches []int64, hasSSLNames bool) (writeOff, readOff, distance int64, ok bool) {
	if !hasSSLNames || len(matches) != 2 {
		return 0, 0, 0, false
	}
	distance = matches[1] - matches[0]
	if distance < 0x100 || distance >= 0x10000 {
		return 0, 0, 0, false
	}
	return matches[0], matches[1], distance, true
}

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

	// Exact BoringSSL patterns remain the preferred stripped-binary path.
	readOff = findBS(data, bsSSLRead.pattern)
	if readOff >= 0 {
		writeCenter := readOff + 0xCA0
		writeOff = findBSNear(data, bsSSLWrite.pattern, writeCenter, 0x10000)
		if writeOff >= 0 {
			log.Printf("[tls] → matched BoringSSL patterns: SSL_read=%#x SSL_write=%#x", readOff, writeOff)
			found = true
		}
	}

	if !found {
		osslMask := buildMask(osslSSLCommonPrefix.pattern)
		var matches []int64
		for i := int64(0); i <= int64(len(data))-int64(len(osslSSLCommonPrefix.pattern)); i++ {
			if matchMasked(data[i:], osslSSLCommonPrefix.pattern, osslMask) {
				matches = append(matches, i)
			}
		}
		log.Printf("[tls] OpenSSL common prologue found at %d locations", len(matches))
		var distance int64
		writeOff, readOff, distance, found = selectUnambiguousOpenSSLPair(matches, binaryContainsSSLReadWriteStrings(data))
		if found {
			log.Printf("[tls] → accepted unambiguous OpenSSL fallback: SSL_write=%#x SSL_read=%#x (dist=%#x)", writeOff, readOff, distance)
		} else if len(matches) > 0 {
			log.Printf("[tls] → refusing ambiguous OpenSSL offset fallback (%d common-prologue candidates)", len(matches))
		}
	}

	if !found {
		return fmt.Errorf("no unambiguous SSL function patterns matched in stripped binary (%d bytes)", len(data))
	}

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
	// SSL_write is captured at function entry by uprobe_ssl_write, so there is
	// intentionally no SSL_write return program. SSL_read must keep entry+return
	// because the return value supplies the actual plaintext byte count.
	if err := attachOffsetProbe(bin, m, "uprobe_ssl_write", uint64(writeOff), false, opts); err != nil {
		errs = append(errs, fmt.Errorf("SSL_write entry: %w", err))
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
	m.registerPIDLinkRangeLocked(pid, startLinks)
	return nil
}

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
		dynSyms, _ := f.DynamicSymbols()
		if len(dynSyms) > 0 {
			log.Printf("[tls]   dynamic symbols (%d):", len(dynSyms))
			for _, s := range dynSyms {
				log.Printf("[tls]     %s", s.Name)
			}
		}
		return
	}

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
