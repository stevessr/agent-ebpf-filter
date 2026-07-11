package tls

import (
	"agent-ebpf-filter/internal/binaryresolver"
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
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
			"/usr/lib/libssl.so.3", "/usr/lib/libssl.so", "/usr/lib/libssl.so.1.1", "/usr/lib/libssl3.so",
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

	readLoopStats readLoopAtomicStats

	mu               sync.Mutex
	closed           bool
	discoveryStarted bool
	discoveryCancel  context.CancelFunc
	discoveryWG      sync.WaitGroup
	reader           *perf.Reader
}

type ReadLoopStats struct {
	TotalFrags     int64
	DroppedFrags   int64
	CompletedFrags int64
	HTTPEvents     int64
	RawEvents      int64
	LastFragmentNS int64
}

type readLoopAtomicStats struct {
	totalFrags     atomic.Int64
	droppedFrags   atomic.Int64
	completedFrags atomic.Int64
	httpEvents     atomic.Int64
	rawEvents      atomic.Int64
	lastFragmentNS atomic.Int64
}

func (s *readLoopAtomicStats) Snapshot() ReadLoopStats {
	if s == nil {
		return ReadLoopStats{}
	}
	return ReadLoopStats{
		TotalFrags:     s.totalFrags.Load(),
		DroppedFrags:   s.droppedFrags.Load(),
		CompletedFrags: s.completedFrags.Load(),
		HTTPEvents:     s.httpEvents.Load(),
		RawEvents:      s.rawEvents.Load(),
		LastFragmentNS: s.lastFragmentNS.Load(),
	}
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
		log.Printf("[tls] attachEntryProbe: SKIP symbol=%q label=%s — no program mapping", symbol, label)
		return nil, nil
	}
	prog, ok := programByName(&m.objs.AgentTlsCapturePrograms, programName)
	if !ok || prog == nil {
		log.Printf("[tls] attachEntryProbe: SKIP symbol=%q label=%s program=%q — program not found in loaded BPF object", symbol, label, programName)
		return nil, nil
	}
	pidHint := ""
	if opts != nil && opts.PID != 0 {
		pidHint = fmt.Sprintf(" pid=%d", opts.PID)
	}
	l, err := executable.Uprobe(symbol, prog, opts)
	if err != nil {
		log.Printf("[tls] attachEntryProbe: FAIL symbol=%q label=%s program=%q%s: %v", symbol, label, programName, pidHint, err)
		return nil, fmt.Errorf("attach %s uprobe %s: %w", label, symbol, err)
	}
	log.Printf("[tls] attachEntryProbe: OK symbol=%q label=%s program=%q%s", symbol, label, programName, pidHint)
	m.links = append(m.links, l)
	return l, nil
}

func (m *TLSProbeManager) attachReturnProbe(executable *link.Executable, label, symbol string, opts *link.UprobeOptions) (link.Link, error) {
	programName, ok := tlsReturnProgramForSymbol(symbol)
	if !ok {
		log.Printf("[tls] attachReturnProbe: SKIP symbol=%q label=%s — no return program mapping", symbol, label)
		return nil, nil
	}
	prog, ok := programByName(&m.objs.AgentTlsCapturePrograms, programName)
	if !ok || prog == nil {
		log.Printf("[tls] attachReturnProbe: SKIP symbol=%q label=%s program=%q — program not found in loaded BPF object", symbol, label, programName)
		return nil, nil
	}
	pidHint := ""
	if opts != nil && opts.PID != 0 {
		pidHint = fmt.Sprintf(" pid=%d", opts.PID)
	}
	l, err := executable.Uretprobe(symbol, prog, opts)
	if err != nil {
		log.Printf("[tls] attachReturnProbe: FAIL symbol=%q label=%s program=%q%s: %v", symbol, label, programName, pidHint, err)
		return nil, fmt.Errorf("attach %s uretprobe %s: %w", label, symbol, err)
	}
	log.Printf("[tls] attachReturnProbe: OK symbol=%q label=%s program=%q%s", symbol, label, programName, pidHint)
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
