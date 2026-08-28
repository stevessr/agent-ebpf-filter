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
	targets, err := parseGoTLSTargets(binPath)
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
	if pid > 0 {
		opts.PID = pid
	}
	startLinks := len(m.links)
	var errs []error
	attachedCount := 0
	for _, target := range targets {
		if programName := tlsProgramForSymbolName(target.Name); programName != "" {
			if err := attachOffsetProbe(bin, m, programName, target.Address, false, opts); err != nil {
				errs = append(errs, fmt.Errorf("%s entry: %w", target.Name, err))
			} else {
				attachedCount++
			}
		}
		if programName := tlsReturnProgramForSymbolName(target.Name); programName != "" {
			if err := attachOffsetProbe(bin, m, programName, target.Address, true, opts); err != nil {
				errs = append(errs, fmt.Errorf("%s return: %w", target.Name, err))
			} else {
				attachedCount++
			}
		}
	}
	if attachedCount == 0 {
		for _, l := range m.links[startLinks:] {
			if l != nil {
				_ = l.Close()
			}
		}
		m.links = m.links[:startLinks]
		if err := errors.Join(errs...); err != nil {
			return err
		}
		return fmt.Errorf("zero Go TLS probes attached to %s", binPath)
	}
	m.registerPIDLinkRangeLocked(pid, startLinks)
	if len(errs) > 0 {
		log.Printf("[tls] AttachGoUprobes: partial coverage for %s (pid=%d): %v", binPath, pid, errors.Join(errs...))
	}
	if m.store != nil {
		status := TLSLibraryStatus{Name: "go", Path: binPath, Attached: true, Available: true}
		if len(errs) > 0 {
			status.Error = "partial probe coverage: " + errors.Join(errs...).Error()
		}
		m.store.SetLibraryStatus(status)
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
	if pid > 0 {
		opts.PID = pid
	}
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
	log.Printf("[tls] AttachStaticSSLUprobes: %d probes attached for %s (pid=%d)", attachedCount, binPath, pid)
	if attachedCount == 0 {
		for _, l := range m.links[startLinks:] {
			if l != nil {
				_ = l.Close()
			}
		}
		m.links = m.links[:startLinks]
		if err := errors.Join(errs...); err != nil {
			return err
		}
		return fmt.Errorf("zero TLS probes attached to %s — symbols may exist but eBPF program lookup failed", binPath)
	}
	m.registerPIDLinkRangeLocked(pid, startLinks)
	if len(errs) > 0 {
		log.Printf("[tls] AttachStaticSSLUprobes: partial coverage for %s (pid=%d): %v", binPath, pid, errors.Join(errs...))
	}
	if m.store != nil {
		status := TLSLibraryStatus{Name: "static-openssl", Path: binPath, Attached: true, Available: true}
		if len(errs) > 0 {
			status.Error = "partial probe coverage: " + errors.Join(errs...).Error()
		}
		m.store.SetLibraryStatus(status)
	}
	return nil
}

// attachLoadedLibraryForPIDLocked attaches a shared TLS library only to the
// process that actually mapped it. The caller must hold m.mu.
func (m *TLSProbeManager) attachLoadedLibraryForPIDLocked(target ProbeTarget, path string, pid int, status TLSLibraryStatus) error {
	if pid <= 0 {
		return m.attachLibraryPathLocked(target, path, status)
	}
	if m.closed || m.objs == nil {
		return fmt.Errorf("TLS probe manager is closed")
	}
	if m.attachedStatic == nil {
		m.attachedStatic = make(map[string]bool)
	}
	attachKey := fmt.Sprintf("pid\x00%d\x00%s\x00%s", pid, target.name, path)
	if m.attachedStatic[attachKey] {
		status.Attached = true
		m.store.SetLibraryStatus(status)
		return nil
	}

	lib, err := link.OpenExecutable(path)
	if err != nil {
		status.Error = err.Error()
		m.store.SetLibraryStatus(status)
		return fmt.Errorf("open %s: %w", path, err)
	}
	opts := &link.UprobeOptions{PID: pid}
	startLinks := len(m.links)
	attached := 0
	var errs []error
	attachSymbol := func(symbol string) {
		if l, err := m.attachEntryProbe(lib, target.name, symbol, opts); err != nil {
			errs = append(errs, err)
		} else if l != nil {
			attached++
		}
		if _, ok := tlsReturnProgramForSymbol(symbol); ok {
			if l, err := m.attachReturnProbe(lib, target.name, symbol, opts); err != nil {
				errs = append(errs, err)
			} else if l != nil {
				attached++
			}
		}
	}
	for _, symbol := range target.sendSymbols {
		attachSymbol(symbol)
	}
	for _, symbol := range target.recvSymbols {
		attachSymbol(symbol)
	}

	if attached == 0 {
		for _, l := range m.links[startLinks:] {
			if l != nil {
				_ = l.Close()
			}
		}
		m.links = m.links[:startLinks]
		if err := errors.Join(errs...); err != nil {
			status.Error = err.Error()
			m.store.SetLibraryStatus(status)
			return err
		}
		err := fmt.Errorf("no TLS probes attached for %s", path)
		status.Error = err.Error()
		m.store.SetLibraryStatus(status)
		return err
	}

	m.registerPIDLinkRangeLocked(pid, startLinks)
	status.Attached = true
	m.attachedStatic[attachKey] = true
	if len(errs) > 0 {
		status.Error = "partial probe coverage: " + errors.Join(errs...).Error()
	}
	m.store.SetLibraryStatus(status)
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
		if pid > 0 {
			m.mu.Lock()
			if m.attachedExec == nil {
				m.attachedExec = make(map[int]string)
			}
			m.attachedExec[pid] = "go-crypto-tls"
			m.mu.Unlock()
		}
		return result
	}

	if err := m.AttachStaticSSLUprobes(attachPath, pid); err == nil {
		log.Printf("[tls] AttachExecutable: static SSL uprobes attached to %s (pid=%d)", attachPath, pid)
		result.TargetKind = "static-ssl"
		result.Library = "static-openssl"
		if pid > 0 {
			m.mu.Lock()
			if m.attachedExec == nil {
				m.attachedExec = make(map[int]string)
			}
			m.attachedExec[pid] = "static-openssl"
			m.mu.Unlock()
		}
		return result
	}
	log.Printf("[tls] AttachExecutable: symbol-based static SSL failed for %s, trying rustls offset detection...", attachPath)

	if err := m.AttachRustlsUprobes(attachPath, pid); err == nil {
		log.Printf("[tls] AttachExecutable: rustls uprobes attached to %s (pid=%d)", attachPath, pid)
		result.TargetKind = "static-ssl"
		result.Library = "rustls"
		if pid > 0 {
			m.mu.Lock()
			if m.attachedExec == nil {
				m.attachedExec = make(map[int]string)
			}
			m.attachedExec[pid] = "rustls"
			m.mu.Unlock()
		}
		return result
	}
	log.Printf("[tls] AttachExecutable: rustls offset detection failed for %s, trying BoringSSL byte-pattern detection...", attachPath)

	if err := m.AttachBoringSSLByOffsets(attachPath, pid); err == nil {
		log.Printf("[tls] AttachExecutable: BoringSSL/OpenSSL attached by absolute offset in %s (pid=%d)", attachPath, pid)
		result.TargetKind = "static-ssl"
		result.Library = "boringssl"
		if pid > 0 {
			m.mu.Lock()
			if m.attachedExec == nil {
				m.attachedExec = make(map[int]string)
			}
			m.attachedExec[pid] = "boringssl"
			m.mu.Unlock()
		}
		return result
	}
	log.Printf("[tls] AttachExecutable: BoringSSL detection also failed for %s", attachPath)

	if hasRustlsStrings(attachPath) {
		log.Printf("[tls] AttachExecutable: %s contains rustls strings but offset detection failed", attachPath)
	}

	loadedLibs := findLoadedSSLLibraries(pid)
	libraries := executableLibraryCandidates(libraryHint)
	var errs []error
	for _, target := range libraries {
		libPath, libOk := findLoadedLibForTarget(loadedLibs, target)
		if !libOk && pid <= 0 {
			libPath, libOk = findFirstExistingPath(target.paths...)
		}
		if !libOk {
			errs = append(errs, fmt.Errorf("process %d has no loaded %s library", pid, target.name))
			continue
		}
		m.mu.Lock()
		status := TLSLibraryStatus{Name: target.name, Path: libPath, Available: true}
		err := m.attachLoadedLibraryForPIDLocked(target, libPath, pid, status)
		if err != nil {
			m.mu.Unlock()
			log.Printf("[tls] AttachExecutable: library %s (%s) attach failed: %v", target.name, libPath, err)
			errs = append(errs, err)
		} else {
			log.Printf("[tls] AttachExecutable: library %s (%s) attached successfully (pid=%d)", target.name, libPath, pid)
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
	if pid <= 0 {
		for _, status := range result.LibraryPaths {
			if status.Attached {
				result.TargetKind = "executable"
				result.Library = status.Name
				return result
			}
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
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		path := fields[len(fields)-1]
		if !strings.HasSuffix(path, ".so") && !strings.Contains(path, ".so.") {
			continue
		}
		base := strings.ToLower(filepath.Base(path))
		for _, prefix := range []string{"libssl", "libgnutls", "libnspr4", "libnss3", "libnssutil3", "libtls", "libbearssl"} {
			if strings.HasPrefix(base, prefix) {
				if !seen[path] {
					seen[path] = true
					libs = append(libs, path)
				}
				break
			}
		}
	}
	return libs
}

func findLoadedLibForTarget(loadedLibs []string, target ProbeTarget) (string, bool) {
	if len(loadedLibs) == 0 {
		return "", false
	}
	for _, lib := range loadedLibs {
		base := strings.ToLower(filepath.Base(lib))
		switch target.name {
		case "openssl":
			if strings.HasPrefix(base, "libssl") {
				return lib, true
			}
		case "gnutls":
			if strings.HasPrefix(base, "libgnutls") {
				return lib, true
			}
		case "nss":
			if strings.HasPrefix(base, "libnspr4") {
				return lib, true
			}
		}
	}
	return "", false
}
