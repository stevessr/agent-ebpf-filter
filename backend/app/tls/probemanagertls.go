package tls

import (
	"agent-ebpf-filter/internal/binaryresolver"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

type ProbeTarget struct {
	name        string
	paths       []string
	sendSymbols []string
	recvSymbols []string
	libType     uint8
}

var staticTLSLibraries = []ProbeTarget{
	{
		name: "openssl",
		paths: []string{
			"/usr/lib/libssl.so.3", "/usr/lib/libssl.so", "/usr/lib/libssl.so.1.1",
			"/lib/x86_64-linux-gnu/libssl.so.3", "/lib/aarch64-linux-gnu/libssl.so.3", "/lib64/libssl.so.3", "/usr/lib64/libssl.so.3", "/usr/lib/x86_64-linux-gnu/libssl.so.3", "/usr/lib/aarch64-linux-gnu/libssl.so.3", "/usr/local/lib/libssl.so.3", "/usr/local/lib64/libssl.so.3",
			"/lib/x86_64-linux-gnu/libssl.so.1.1", "/lib/aarch64-linux-gnu/libssl.so.1.1", "/lib64/libssl.so.1.1", "/usr/lib64/libssl.so.1.1", "/usr/lib/x86_64-linux-gnu/libssl.so.1.1", "/usr/lib/aarch64-linux-gnu/libssl.so.1.1", "/usr/local/lib/libssl.so.1.1", "/usr/local/lib64/libssl.so.1.1",
			"/lib/x86_64-linux-gnu/libssl.so", "/lib/aarch64-linux-gnu/libssl.so", "/lib64/libssl.so", "/usr/lib64/libssl.so", "/usr/lib/x86_64-linux-gnu/libssl.so", "/usr/lib/aarch64-linux-gnu/libssl.so", "/usr/local/lib/libssl.so", "/usr/local/lib64/libssl.so",
		},
		sendSymbols: []string{"SSL_write", "SSL_write_ex"},
		recvSymbols: []string{"SSL_read", "SSL_read_ex"},
		libType:     tlsLibOpenSSL,
	},
	{
		name:        "gnutls",
		paths:       []string{"/lib/x86_64-linux-gnu/libgnutls.so.30", "/lib/aarch64-linux-gnu/libgnutls.so.30", "/lib64/libgnutls.so.30", "/usr/lib64/libgnutls.so.30", "/usr/lib/x86_64-linux-gnu/libgnutls.so.30", "/usr/lib/aarch64-linux-gnu/libgnutls.so.30", "/usr/lib/libgnutls.so.30", "/usr/local/lib/libgnutls.so.30", "/usr/local/lib64/libgnutls.so.30"},
		sendSymbols: []string{"gnutls_record_send"},
		recvSymbols: []string{"gnutls_record_recv"},
		libType:     tlsLibGnuTLS,
	},
	{
		name:        "nss",
		paths:       []string{"/lib/x86_64-linux-gnu/libnspr4.so", "/lib/aarch64-linux-gnu/libnspr4.so", "/lib64/libnspr4.so", "/usr/lib64/libnspr4.so", "/usr/lib/x86_64-linux-gnu/libnspr4.so", "/usr/lib/aarch64-linux-gnu/libnspr4.so", "/usr/lib/libnspr4.so", "/usr/local/lib/libnspr4.so", "/usr/local/lib64/libnspr4.so"},
		sendSymbols: []string{"PR_Write"},
		recvSymbols: []string{"PR_Read"},
		libType:     tlsLibNSS,
	},
}

func resolveManualTLSProbeTarget(path, library string) (ProbeTarget, error) {
	lookup := strings.ToLower(strings.TrimSpace(library))
	if lookup == "" {
		lookup = strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	}
	for _, target := range staticTLSLibraries {
		if lookup == target.name || strings.Contains(lookup, target.name) {
			return target, nil
		}
	}
	if strings.Contains(lookup, "libssl") {
		return staticTLSLibraries[0], nil
	}
	if strings.Contains(lookup, "libgnutls") {
		return staticTLSLibraries[1], nil
	}
	if strings.Contains(lookup, "libnspr") || strings.Contains(lookup, "libnss") {
		return staticTLSLibraries[2], nil
	}
	return ProbeTarget{}, fmt.Errorf("unsupported TLS library %q", lookup)
}

type TLSProbeManager struct {
	objs           *bpf.AgentTlsCaptureObjects
	links          []link.Link
	assembler      *FragmentAssembler
	httpStreams    *TLSHTTPStreamAssembler
	store          *TLSCaptureStore
	rules          *TLSCaptureRuleStore
	broadcaster    *TLSBroadcaster
	attachedStatic map[string]bool
	attachedGo     map[string]bool
	attachedExec   map[int]string // PID → library name for executable/library attaches

	mu     sync.Mutex
	closed bool
}

type TLSExecutableAttachResult struct {
	Resolved     binaryresolver.ResolvedBinary `json:"resolved"`
	AttachPath   string                        `json:"attachPath,omitempty"`
	TargetKind   string                        `json:"targetKind,omitempty"`
	Library      string                        `json:"library,omitempty"`
	PID          int                           `json:"pid,omitempty"`
	StaticTLS    bool                          `json:"staticTls,omitempty"`
	LibraryPaths []TLSLibraryStatus            `json:"libraryPaths,omitempty"`
	Error        string                        `json:"error,omitempty"`
}

func NewTLSProbeManager(store *TLSCaptureStore, broadcaster *TLSBroadcaster, rules *TLSCaptureRuleStore) (*TLSProbeManager, error) {
	objs := &bpf.AgentTlsCaptureObjects{}
	if err := bpf.LoadAgentTlsCaptureObjects(objs, nil); err != nil {
		return nil, err
	}
	if store == nil {
		store = NewTLSCaptureStore(1000)
	}
	if broadcaster == nil {
		broadcaster = NewTLSCaptureBroadcaster()
	}
	if rules == nil {
		rules = NewTLSCaptureRuleStore()
	}
	return &TLSProbeManager{
		objs:           objs,
		assembler:      NewFragmentAssembler(10 * time.Second),
		httpStreams:    NewTLSHTTPStreamAssembler(10 * time.Second),
		store:          store,
		rules:          rules,
		broadcaster:    broadcaster,
		attachedStatic: make(map[string]bool),
		attachedGo:     make(map[string]bool),
		attachedExec:   make(map[int]string),
	}, nil
}

func (m *TLSProbeManager) AttachStaticLibs() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.objs == nil {
		return fmt.Errorf("TLS probe manager is closed")
	}
	var errs []error
	for _, target := range staticTLSLibraries {
		path, ok := findFirstExistingPath(target.paths...)
		status := TLSLibraryStatus{Name: target.name, Path: path}
		if !ok {
			status.Available = false
			status.Attached = false
			status.Error = "library not found"
			m.store.SetLibraryStatus(status)
			continue
		}
		if err := m.attachLibraryPathLocked(target, path, status); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *TLSProbeManager) AttachLibrary(path, library string) error {
	if m == nil {
		return nil
	}
	target, err := resolveManualTLSProbeTarget(path, library)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.objs == nil {
		return fmt.Errorf("TLS probe manager is closed")
	}
	return m.attachLibraryPathLocked(target, strings.TrimSpace(path), TLSLibraryStatus{Name: target.name, Path: strings.TrimSpace(path), Available: true})
}

func (m *TLSProbeManager) attachLibraryPathLocked(target ProbeTarget, path string, status TLSLibraryStatus) error {
	status.Available = true
	attachKey := target.name + "\x00" + path
	if m.attachedStatic[attachKey] {
		status.Attached = true
		m.store.SetLibraryStatus(status)
		return nil
	}
	attached, err := m.attachLibraryPath(target, path, status)
	status.Attached = attached > 0
	if status.Attached {
		m.attachedStatic[attachKey] = true
	}
	if err != nil {
		status.Error = err.Error()
	}
	if attached == 0 && err == nil {
		err = fmt.Errorf("no TLS probes attached for %s", path)
		status.Error = err.Error()
	}
	m.store.SetLibraryStatus(status)
	return err
}

func (m *TLSProbeManager) attachLibraryPath(target ProbeTarget, path string, status TLSLibraryStatus) (int, error) {
	if m == nil || m.objs == nil {
		return 0, nil
	}
	lib, err := link.OpenExecutable(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}

	attached := 0
	var errs []error
	for _, symbol := range target.sendSymbols {
		if l, err := m.attachEntryProbe(lib, target.name, symbol, nil); err != nil {
			errs = append(errs, err)
		} else if l != nil {
			attached++
		}
		if l, err := m.attachReturnProbe(lib, target.name, symbol, nil); err != nil {
			errs = append(errs, err)
		} else if l != nil {
			attached++
		}
	}
	for _, symbol := range target.recvSymbols {
		if l, err := m.attachEntryProbe(lib, target.name, symbol, nil); err != nil {
			errs = append(errs, err)
		} else if l != nil {
			attached++
		}
		if l, err := m.attachReturnProbe(lib, target.name, symbol, nil); err != nil {
			errs = append(errs, err)
		} else if l != nil {
			attached++
		}
	}
	return attached, errors.Join(errs...)
}

func (m *TLSProbeManager) attachEntryProbe(executable *link.Executable, label, symbol string, opts *link.UprobeOptions) (link.Link, error) {
	programName, ok := tlsProgramForSymbol(symbol)
	if !ok {
		return nil, nil
	}
	prog, ok := programByName(&m.objs.AgentTlsCapturePrograms, programName)
	if !ok || prog == nil {
		return nil, nil
	}
	l, err := executable.Uprobe(symbol, prog, opts)
	if err != nil {
		return nil, fmt.Errorf("attach %s uprobe %s: %w", label, symbol, err)
	}
	m.links = append(m.links, l)
	return l, nil
}

func (m *TLSProbeManager) attachReturnProbe(executable *link.Executable, label, symbol string, opts *link.UprobeOptions) (link.Link, error) {
	programName, ok := tlsReturnProgramForSymbol(symbol)
	if !ok {
		return nil, nil
	}
	prog, ok := programByName(&m.objs.AgentTlsCapturePrograms, programName)
	if !ok || prog == nil {
		return nil, nil
	}
	l, err := executable.Uretprobe(symbol, prog, opts)
	if err != nil {
		return nil, fmt.Errorf("attach %s uretprobe %s: %w", label, symbol, err)
	}
	m.links = append(m.links, l)
	return l, nil
}

func executableTLSAttachPath(resolved binaryresolver.ResolvedBinary) string {
	if resolved.Shebang != "" {
		if interpreter := resolveShebangInterpreter(resolved.Shebang); interpreter != "" {
			return interpreter
		}
	}
	if resolved.RealPath != "" {
		return resolved.RealPath
	}
	return resolved.Path
}

func resolveShebangInterpreter(shebang string) string {
	fields := strings.Fields(strings.TrimSpace(shebang))
	if len(fields) == 0 {
		return ""
	}
	command := fields[0]
	if filepath.Base(command) == "env" {
		for _, field := range fields[1:] {
			if strings.HasPrefix(field, "-") || strings.Contains(field, "=") {
				continue
			}
			command = field
			break
		}
	}
	if command == "" || filepath.Base(command) == "env" {
		return ""
	}
	resolved := binaryresolver.ResolveBinary(command, "")
	if resolved.Error == "" {
		if resolved.RealPath != "" {
			return resolved.RealPath
		}
		return resolved.Path
	}
	if filepath.IsAbs(command) {
		return command
	}
	return ""
}

func executableLibraryCandidates(libraryHint string) []ProbeTarget {
	libraryHint = strings.TrimSpace(libraryHint)
	if libraryHint == "" || strings.EqualFold(libraryHint, "auto") {
		return staticTLSLibraries
	}
	target, err := resolveManualTLSProbeTarget("", libraryHint)
	if err != nil {
		return staticTLSLibraries
	}
	return []ProbeTarget{target}
}

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
	if pid > 0 {
		opts.PID = pid
	}
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
	if pid > 0 {
		opts.PID = pid
	}
	startLinks := len(m.links)
	var errs []error
	staticSymbols := []string{"SSL_write", "SSL_write_ex", "SSL_read", "SSL_read_ex"}
	for _, sym := range staticSymbols {
		if _, err := m.attachEntryProbe(bin, "static-openssl", sym, opts); err != nil {
			errs = append(errs, err)
		}
		if _, ok := tlsReturnProgramForSymbol(sym); ok {
			if _, err := m.attachReturnProbe(bin, "static-openssl", sym, opts); err != nil {
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

	libraries := executableLibraryCandidates(libraryHint)
	var errs []error
	for _, target := range libraries {
		// Resolve the actual library .so path on the system, NOT the executable path.
		libPath, libOk := findFirstExistingPath(target.paths...)
		if !libOk {
			errs = append(errs, fmt.Errorf("library %s not found on system", target.name))
			continue
		}
		m.mu.Lock()
		status := TLSLibraryStatus{Name: target.name, Path: libPath, Available: true}
		err := m.attachLibraryPathLocked(target, libPath, status)
		if err != nil {
			m.mu.Unlock()
			errs = append(errs, err)
		} else {
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

func (m *TLSProbeManager) ReadLoop() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.closed || m.objs == nil || m.objs.TlsEvents == nil || m.assembler == nil || m.httpStreams == nil || m.store == nil || m.broadcaster == nil {
		m.mu.Unlock()
		return nil
	}
	events := m.objs.TlsEvents
	assembler := m.assembler
	httpStreams := m.httpStreams
	store := m.store
	broadcaster := m.broadcaster
	m.mu.Unlock()

	reader, err := ringbuf.NewReader(events)
	if err != nil {
		return err
	}
	defer reader.Close()

	for {
		rec, err := reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return nil
			}
			return err
		}
		var fragment tlsFragment
		if err := binary.Read(bytes.NewReader(rec.RawSample), binary.LittleEndian, &fragment); err != nil {
			continue
		}
		completed, ok := assembler.Add(fragment)
		if !ok || completed == nil {
			continue
		}
		for _, event := range httpStreams.Add(*completed) {
			if m.rules != nil && !m.rules.Allows(event) {
				continue
			}
			DispatchTLSAgentEvent(&event, tlsAgentLoopDetector, deps.Broadcast)
			store.Add(event)
			broadcaster.Broadcast(event)
		}
	}
}

func (m *TLSProbeManager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	links := m.links
	m.links = nil
	objs := m.objs
	m.objs = nil
	m.mu.Unlock()

	var errs []error
	for _, l := range links {
		if l == nil {
			continue
		}
		if err := l.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if objs != nil {
		if err := objs.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func programByName(programs *bpf.AgentTlsCapturePrograms, name string) (*ebpf.Program, bool) {
	if programs == nil || name == "" {
		return nil, false
	}
	programsByName := map[string]*ebpf.Program{
		"uprobe_crypto_tls_conn_read":    programs.UprobeCryptoTlsConnRead,
		"uprobe_crypto_tls_conn_write":   programs.UprobeCryptoTlsConnWrite,
		"uprobe_gnutls_record_recv":      programs.UprobeGnutlsRecordRecv,
		"uprobe_gnutls_record_send":      programs.UprobeGnutlsRecordSend,
		"uprobe_pr_read":                 programs.UprobePrRead,
		"uprobe_pr_write":                programs.UprobePrWrite,
		"uprobe_ssl_read":                programs.UprobeSslRead,
		"uprobe_ssl_read_ex":             programs.UprobeSslReadEx,
		"uprobe_ssl_write":               programs.UprobeSslWrite,
		"uprobe_ssl_write_ex":            programs.UprobeSslWriteEx,
		"uretprobe_crypto_tls_conn_read": programs.UretprobeCryptoTlsConnRead,
		"uretprobe_gnutls_record_recv":   programs.UretprobeGnutlsRecordRecv,
		"uretprobe_pr_read":              programs.UretprobePrRead,
		"uretprobe_ssl_read":             programs.UretprobeSslRead,
		"uretprobe_ssl_read_ex":          programs.UretprobeSslReadEx,
		"uretprobe_ssl_write_ex":         programs.UretprobeSslWriteEx,
	}
	prog, ok := programsByName[name]
	return prog, ok
}

// AttachedPIDInfo describes a process that has an active SSL/TLS uprobe.
type AttachedPIDInfo struct {
	PID         int    `json:"pid"`
	BinaryPath  string `json:"binary_path"`
	LibraryName string `json:"library_name"`
}

// AttachedPIDs returns the list of PIDs that currently have SSL/TLS uprobes
// attached, derived from both attachedGo (Go uprobes) and attachedExec
// (library/executable attaches).
func (m *TLSProbeManager) AttachedPIDs() []AttachedPIDInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	seen := make(map[int]bool)
	var result []AttachedPIDInfo

	// Go crypto/tls uprobes
	for key := range m.attachedGo {
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		pid := 0
		fmt.Sscanf(parts[0], "%d", &pid)
		if pid > 0 && !seen[pid] {
			seen[pid] = true
			lib := "go-crypto-tls"
			if strings.Contains(parts[1], "node") || strings.Contains(parts[1], "bun") || strings.Contains(parts[1], "deno") {
				lib = "openssl"
			}
			result = append(result, AttachedPIDInfo{
				PID:         pid,
				BinaryPath:  parts[1],
				LibraryName: lib,
			})
		}
	}

	// Library/executable attaches (openssl, gnutls, nss, etc.)
	for pid, lib := range m.attachedExec {
		if pid > 0 && !seen[pid] {
			seen[pid] = true
			// Get binary path from /proc
			binPath := ""
			if link, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
				binPath = link
			}
			result = append(result, AttachedPIDInfo{
				PID:         pid,
				BinaryPath:  binPath,
				LibraryName: lib,
			})
		}
	}

	return result
}
