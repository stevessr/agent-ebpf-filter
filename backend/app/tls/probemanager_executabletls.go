package tls

import (
	"agent-ebpf-filter/internal/binaryresolver"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cilium/ebpf/link"
)

func (m *TLSProbeManager) AttachGoUprobes(binPath string, pid int) error {
	if m == nil {
		return nil
	}
	parsed, err := parseGoTLSSymbols(binPath)
	if err != nil {
		return err
	}

	bin, err := link.OpenExecutable(binPath)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.objs == nil {
		return fmt.Errorf("TLS probe manager is closed")
	}
	opts := &link.UprobeOptions{}
	// Kernel 7.1+ PID-specific uprobe workaround — see AttachStaticSSLUprobes.
	_ = pid
	startLinks := len(m.links)
	var errs []error
	for _, sym := range parsed {
		if _, err := m.attachEntryProbe(bin, "go", sym, opts); err != nil {
			errs = append(errs, err)
		}
		if _, ok := tlsReturnProgramForSymbol(sym); ok {
			if _, err := m.attachReturnProbe(bin, "go", sym, opts); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		for _, l := range m.links[startLinks:] {
			if l != nil {
				_ = l.Close()
			}
		}
		m.links = m.links[:startLinks]
		return err
	}
	if m.store != nil {
		m.store.SetLibraryStatus(TLSLibraryStatus{Name: "go", Path: binPath, Attached: true, Available: true})
	}
	return nil
}

func (m *TLSProbeManager) AttachStaticSSLUprobes(binPath string, pid int) error {
	if m == nil {
		return nil
	}
	bin, err := link.OpenExecutable(binPath)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.objs == nil {
		return fmt.Errorf("TLS probe manager is closed")
	}
	opts := &link.UprobeOptions{}
	// Kernel 7.1+ appears to have issues with PID-specific uprobes.
	// Use global uprobes (PID=0) as a workaround — the probe fires for
	// all processes that map this binary, not just the target PID.
	_ = pid // keep signature; TODO: re-enable PID filter when kernel compat is confirmed
	startLinks := len(m.links)
	var errs []error
	staticSymbols := []string{"SSL_write", "SSL_write_ex", "SSL_read", "SSL_read_ex", "SSL_write_ex2"}
	attachedCount := 0
	for _, sym := range staticSymbols {
		if l, err := m.attachEntryProbe(bin, "static-openssl", sym, opts); err != nil {
			errs = append(errs, err)
		} else if l != nil {
			attachedCount++
		}
		if _, ok := tlsReturnProgramForSymbol(sym); ok {
			if l, err := m.attachReturnProbe(bin, "static-openssl", sym, opts); err != nil {
				errs = append(errs, err)
			} else if l != nil {
				attachedCount++
			}
		}
	}
	log.Printf("[tls] AttachStaticSSLUprobes: %d/%d probes attached for %s (pid=%d)",
		attachedCount, len(staticSymbols)*2, binPath, pid)
	if err := errors.Join(errs...); err != nil {
		for _, l := range m.links[startLinks:] {
			if l != nil {
				_ = l.Close()
			}
		}
		m.links = m.links[:startLinks]
		return err
	}
	if attachedCount == 0 {
		return fmt.Errorf("zero TLS probes attached to %s — symbols may exist but eBPF program lookup failed", binPath)
	}
	if m.store != nil {
		m.store.SetLibraryStatus(TLSLibraryStatus{Name: "static-openssl", Path: binPath, Attached: true, Available: true})
	}
	return nil
}

func (m *TLSProbeManager) AttachExecutable(input string, pid int, libraryHint string) TLSExecutableAttachResult {
	result := TLSExecutableAttachResult{PID: pid}
	if m == nil {
		result.Error = "TLS probe manager is unavailable"
		return result
	}
	resolved := binaryresolver.ResolveBinary(input, "")
	result.Resolved = resolved
	if resolved.Error != "" {
		result.Error = resolved.Error
		return result
	}

	attachPath := executableTLSAttachPath(resolved)
	if attachPath == "" {
		attachPath = resolved.RealPath
	}
	result.AttachPath = attachPath
	result.StaticTLS = resolved.StaticTLS

	if err := m.AttachGoUprobes(attachPath, pid); err == nil {
		result.TargetKind = "go"
		result.Library = "go"
		// Track PID for Go uprobes (already done by shouldAttachGoBinary, but ensure)
		m.mu.Lock()
		if m.attachedExec == nil {
			m.attachedExec = make(map[int]string)
		}
		m.attachedExec[pid] = "go-crypto-tls"
		m.mu.Unlock()
		return result
	}

	// Always try static SSL uprobes on the executable itself — this handles
	// statically-linked SSL (Node.js/BoringSSL, Python, etc.) where no
	// dynamic libssl.so is loaded.
	if err := m.AttachStaticSSLUprobes(attachPath, pid); err == nil {
		log.Printf("[tls] AttachExecutable: static SSL uprobes attached to %s (pid=%d)", attachPath, pid)
		result.TargetKind = "static-ssl"
		result.Library = "static-openssl"
		m.mu.Lock()
		if m.attachedExec == nil {
			m.attachedExec = make(map[int]string)
		}
		m.attachedExec[pid] = "static-openssl"
		m.mu.Unlock()
		return result
	}
	log.Printf("[tls] AttachExecutable: symbol-based static SSL failed for %s, trying rustls offset detection...", attachPath)

	// Try rustls BEFORE the BoringSSL byte-pattern heuristic for binaries known
	// to use rustls (codex/cursor — stripped static-pie Rust). The BoringSSL
	// heuristic greps for the "SSL_write" string (present in rustls error/feature
	// strings) and pattern-matches OpenSSL prologues that coincidentally exist
	// elsewhere in .text, attaching uprobes at wrong offsets and returning
	// success — which prevents the precise rustls .eh_frame probe from ever
	// running. So for rustls binaries, try the precise offset probe first.
	if err := m.AttachRustlsUprobes(attachPath, pid); err == nil {
		log.Printf("[tls] AttachExecutable: rustls uprobes attached to %s (pid=%d)", attachPath, pid)
		result.TargetKind = "static-ssl"
		result.Library = "rustls"
		m.mu.Lock()
		if m.attachedExec == nil {
			m.attachedExec = make(map[int]string)
		}
		m.attachedExec[pid] = "rustls"
		m.mu.Unlock()
		return result
	}
	log.Printf("[tls] AttachExecutable: rustls offset detection failed for %s, trying BoringSSL byte-pattern detection...", attachPath)

	// Try BoringSSL byte-pattern detection for stripped binaries
	// (Node.js with BoringSSL, Bun, Claude CLI, etc.)
	if err := m.AttachBoringSSLByOffsets(attachPath, pid); err == nil {
		log.Printf("[tls] AttachExecutable: BoringSSL detected and attached by offset in %s (pid=%d)", attachPath, pid)
		result.TargetKind = "static-ssl"
		result.Library = "boringssl"
		m.mu.Lock()
		if m.attachedExec == nil {
			m.attachedExec = make(map[int]string)
		}
		m.attachedExec[pid] = "boringssl"
		m.mu.Unlock()
		return result
	}
	log.Printf("[tls] AttachExecutable: BoringSSL detection also failed for %s", attachPath)

	// Rustls already attempted above; if we reach here it failed.
	// Check if this looks like a Rust binary via .rodata strings
	if hasRustlsStrings(attachPath) {
		log.Printf("[tls] AttachExecutable: %s contains rustls strings but offset detection failed — byte-pattern heuristics need improvement", attachPath)
		// Fall through to dynamic library attempt (which will also fail for static binaries)
		// but at least we've diagnosed the situation clearly.
	}

	// Dump binary symbols for diagnosis
	dumpCandidateTLSSymbols(attachPath)

	// Find which SSL/TLS libraries this PID actually has loaded via /proc/PID/maps.
	// Attaching to a .so file on disk is useless if the process never mmap'd it.
	loadedLibs := findLoadedSSLLibraries(pid)

	libraries := executableLibraryCandidates(libraryHint)
	var errs []error
	for _, target := range libraries {
		// Prefer the actual loaded library path for this PID, fall back to
		// the first existing path on the system (for system-wide attaches).
		libPath, libOk := findLoadedLibForTarget(loadedLibs, target)
		if !libOk {
			libPath, libOk = findFirstExistingPath(target.paths...)
		}
		if !libOk {
			errs = append(errs, fmt.Errorf("library %s not found on system", target.name))
			continue
		}
		m.mu.Lock()
		status := TLSLibraryStatus{Name: target.name, Path: libPath, Available: true}
		err := m.attachLibraryPathLocked(target, libPath, status)
		if err != nil {
			m.mu.Unlock()
			log.Printf("[tls] AttachExecutable: library %s (%s) attach failed: %v", target.name, libPath, err)
			errs = append(errs, err)
		} else {
			log.Printf("[tls] AttachExecutable: library %s (%s) attached successfully (loaded=%v)", target.name, libPath, loadedLibs != nil)
			if pid > 0 {
				if m.attachedExec == nil {
					m.attachedExec = make(map[int]string)
				}
				m.attachedExec[pid] = target.name
			}
			m.mu.Unlock()
		}
	}
	if m.store != nil {
		result.LibraryPaths = m.store.LibraryStatuses()
	}
	for _, status := range result.LibraryPaths {
		if status.Attached {
			result.TargetKind = "executable"
			result.Library = status.Name
			return result
		}
	}
	// Check PID-tracking map — library uprobe may have succeeded even if
	// the store filter above didn't find it (library path ≠ executable path).
	if pid > 0 {
		m.mu.Lock()
		lib, ok := m.attachedExec[pid]
		m.mu.Unlock()
		if ok {
			result.TargetKind = "executable"
			result.Library = lib
			return result
		}
	}
	if err := errors.Join(errs...); err != nil {
		result.Error = err.Error()
	} else {
		result.Error = fmt.Sprintf("no TLS probes attached for executable %s", attachPath)
	}
	return result
}

func findLoadedSSLLibraries(pid int) []string {
	if pid <= 0 {
		return nil
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		log.Printf("[tls] PID %d: cannot read maps: %v", pid, err)
		return nil
	}
	var libs []string
	seen := make(map[string]bool)
	allSos := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		path := fields[len(fields)-1]
		if !strings.HasSuffix(path, ".so") && !strings.Contains(path, ".so.") {
			continue
		}
		allSos = append(allSos, path)
		base := filepath.Base(path)
		// Match known SSL/TLS library names
		for _, prefix := range []string{"libssl", "libcrypto", "libgnutls", "libnspr4", "libnss3", "libnssutil3", "libtls", "libbearssl"} {
			if strings.HasPrefix(base, prefix) {
				if !seen[path] {
					seen[path] = true
					libs = append(libs, path)
					log.Printf("[tls] PID %d loaded SSL lib: %s", pid, path)
				}
				break
			}
		}
	}
	// Dump ALL loaded .so files for diagnosis
	if len(libs) == 0 {
		log.Printf("[tls] PID %d: NO known SSL lib found among %d loaded .so files:", pid, len(allSos))
		for _, so := range allSos {
			log.Printf("[tls]   PID %d loaded: %s", pid, so)
		}
	}
	return libs
}

// findLoadedLibForTarget checks if any of the loaded library paths match
// the given ProbeTarget (by library name prefix).
func findLoadedLibForTarget(loadedLibs []string, target ProbeTarget) (string, bool) {
	if len(loadedLibs) == 0 {
		return "", false
	}
	for _, lib := range loadedLibs {
		base := strings.ToLower(filepath.Base(lib))
		switch target.name {
		case "openssl":
			if strings.HasPrefix(base, "libssl") || strings.HasPrefix(base, "libcrypto") {
				return lib, true
			}
		case "gnutls":
			if strings.HasPrefix(base, "libgnutls") {
				return lib, true
			}
		case "nss":
			if strings.HasPrefix(base, "libnspr4") || strings.HasPrefix(base, "libnss3") || strings.HasPrefix(base, "libnssutil3") {
				return lib, true
			}
		}
	}
	return "", false
}

// SSL function prologue byte patterns for stripped binaries.
// First byte of each pattern must match exactly; '0x00' bytes in the
// pattern act as wildcards (match any byte at that position).
// Derived from AgentSight's bpf/sslsniff.c + OpenSSL 3.x disassembly.
