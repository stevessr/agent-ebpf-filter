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
	TotalFrags          int64   `json:"totalFrags"`
	DroppedFrags        int64   `json:"droppedFrags"`
	CompletedFrags      int64   `json:"completedFrags"`
	HTTPEvents          int64   `json:"httpEvents"`
	RawEvents           int64   `json:"rawEvents"`
	LastFragmentNS      int64   `json:"lastFragmentNs"`
	PerfSampleBytes     int64   `json:"perfSampleBytes"`
	CompactSamples      int64   `json:"compactSamples"`
	LegacySamples       int64   `json:"legacySamples"`
	MaxSampleBytes      int64   `json:"maxSampleBytes"`
	AverageSampleBytes  float64 `json:"averageSampleBytes"`
}

type readLoopAtomicStats struct {
	totalFrags      atomic.Int64
	droppedFrags    atomic.Int64
	completedFrags  atomic.Int64
	httpEvents      atomic.Int64
	rawEvents       atomic.Int64
	lastFragmentNS  atomic.Int64
	perfSampleBytes atomic.Int64
	compactSamples  atomic.Int64
	legacySamples   atomic.Int64
	maxSampleBytes  atomic.Int64
}

func (s *readLoopAtomicStats) recordSample(rawLen int, compact bool) {
	if s == nil || rawLen <= 0 {
		return
	}
	s.perfSampleBytes.Add(int64(rawLen))
	if compact {
		s.compactSamples.Add(1)
	} else {
		s.legacySamples.Add(1)
	}
	candidate := int64(rawLen)
	for current := s.maxSampleBytes.Load(); candidate > current; current = s.maxSampleBytes.Load() {
		if s.maxSampleBytes.CompareAndSwap(current, candidate) {
			break
		}
	}
}

func (s *readLoopAtomicStats) Snapshot() ReadLoopStats {
	if s == nil {
		return ReadLoopStats{}
	}
	total := s.totalFrags.Load()
	bytes := s.perfSampleBytes.Load()
	average := float64(0)
	if total > 0 {
		average = float64(bytes) / float64(total)
	}
	return ReadLoopStats{
		TotalFrags:         total,
		DroppedFrags:       s.droppedFrags.Load(),
		CompletedFrags:     s.completedFrags.Load(),
		HTTPEvents:         s.httpEvents.Load(),
		RawEvents:          s.rawEvents.Load(),
		LastFragmentNS:     s.lastFragmentNS.Load(),
		PerfSampleBytes:    bytes,
		CompactSamples:     s.compactSamples.Load(),
		LegacySamples:      s.legacySamples.Load(),
		MaxSampleBytes:     s.maxSampleBytes.Load(),
		AverageSampleBytes: average,
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

	attached, attachErr := m.attachLibraryPath(target, path, status)
	status.Attached = attached > 0
	if attached > 0 {
		m.attachedStatic[attachKey] = true
		if attachErr != nil {
			status.Error = "partial probe coverage: " + attachErr.Error()
		}
		m.store.SetLibraryStatus(status)
		return nil
	}

	if attachErr == nil {
		attachErr = fmt.Errorf("no TLS probes attached for %s", path)
	}
	status.Error = attachErr.Error()
	m.store.SetLibraryStatus(status)
	return attachErr
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
			m.links = append(m.links, l)
			attached++
		}
	}
	for _, symbol := range target.recvSymbols {
		if entry, err := m.attachEntryProbe(lib, target.name, symbol, nil); err != nil {
			errs = append(errs, err)
		} else if entry != nil {
			m.links = append(m.links, entry)
			attached++
		}
		if ret, err := m.attachReturnProbe(lib, target.name, symbol, nil); err != nil {
			errs = append(errs, err)
		} else if ret != nil {
			m.links = append(m.links, ret)
			attached++
		}
	}
	return attached, errors.Join(errs...)
}

// The remaining probe attachment helpers live in the companion manager files.
