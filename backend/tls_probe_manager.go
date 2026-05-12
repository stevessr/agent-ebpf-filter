package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

type tlsProbeTarget struct {
	name       string
	paths      []string
	sendSymbol string
	recvSymbol string
	libType    uint8
}

var staticTLSLibraries = []tlsProbeTarget{
	{
		name:       "openssl",
		paths:      []string{"/lib/x86_64-linux-gnu/libssl.so.3", "/lib64/libssl.so.3", "/usr/lib64/libssl.so.3", "/usr/lib/x86_64-linux-gnu/libssl.so.3", "/usr/lib/libssl.so.3"},
		sendSymbol: "SSL_write",
		recvSymbol: "SSL_read",
		libType:    tlsLibOpenSSL,
	},
	{
		name:       "gnutls",
		paths:      []string{"/lib/x86_64-linux-gnu/libgnutls.so.30", "/lib64/libgnutls.so.30", "/usr/lib64/libgnutls.so.30", "/usr/lib/x86_64-linux-gnu/libgnutls.so.30", "/usr/lib/libgnutls.so.30"},
		sendSymbol: "gnutls_record_send",
		recvSymbol: "gnutls_record_recv",
		libType:    tlsLibGnuTLS,
	},
	{
		name:       "nss",
		paths:      []string{"/lib/x86_64-linux-gnu/libnspr4.so", "/lib64/libnspr4.so", "/usr/lib64/libnspr4.so", "/usr/lib/x86_64-linux-gnu/libnspr4.so", "/usr/lib/libnspr4.so"},
		sendSymbol: "PR_Write",
		recvSymbol: "PR_Read",
		libType:    tlsLibNSS,
	},
}

type TLSProbeManager struct {
	objs        *bpf.AgentTlsCaptureObjects
	links       []link.Link
	assembler   *FragmentAssembler
	store       *TLSCaptureStore
	broadcaster *tlsCaptureBroadcaster
	attachedGo  map[string]bool

	mu     sync.Mutex
	closed bool
}

func NewTLSProbeManager(store *TLSCaptureStore, broadcaster *tlsCaptureBroadcaster) (*TLSProbeManager, error) {
	objs := &bpf.AgentTlsCaptureObjects{}
	if err := bpf.LoadAgentTlsCaptureObjects(objs, nil); err != nil {
		return nil, err
	}
	if store == nil {
		store = NewTLSCaptureStore(1000)
	}
	if broadcaster == nil {
		broadcaster = newTLSCaptureBroadcaster()
	}
	return &TLSProbeManager{
		objs:        objs,
		assembler:   NewFragmentAssembler(10 * time.Second),
		store:       store,
		broadcaster: broadcaster,
		attachedGo:  make(map[string]bool),
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
		status.Available = true
		attached, err := m.attachLibraryPath(target, path, status)
		status.Attached = attached > 0
		if err != nil {
			errs = append(errs, err)
			status.Error = err.Error()
		}
		m.store.SetLibraryStatus(status)
	}
	return errors.Join(errs...)
}

func (m *TLSProbeManager) attachLibraryPath(target tlsProbeTarget, path string, status TLSLibraryStatus) (int, error) {
	if m == nil || m.objs == nil {
		return 0, nil
	}
	lib, err := link.OpenExecutable(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}

	attached := 0
	var errs []error
	if l, err := m.attachEntryProbe(lib, target.name, target.sendSymbol, nil); err != nil {
		errs = append(errs, err)
	} else if l != nil {
		attached++
	}
	if l, err := m.attachEntryProbe(lib, target.name, target.recvSymbol, nil); err != nil {
		errs = append(errs, err)
	} else if l != nil {
		attached++
	}
	if l, err := m.attachReturnProbe(lib, target.name, target.recvSymbol, nil); err != nil {
		errs = append(errs, err)
	} else if l != nil {
		attached++
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
	return nil
}

func (m *TLSProbeManager) ReadLoop() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.closed || m.objs == nil || m.objs.TlsEvents == nil || m.assembler == nil || m.store == nil || m.broadcaster == nil {
		m.mu.Unlock()
		return nil
	}
	events := m.objs.TlsEvents
	assembler := m.assembler
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
		event := parseTLSPlaintext(*completed)
		store.Add(event)
		broadcaster.Broadcast(event)
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
		"uprobe_ssl_write":               programs.UprobeSslWrite,
		"uretprobe_crypto_tls_conn_read": programs.UretprobeCryptoTlsConnRead,
		"uretprobe_gnutls_record_recv":   programs.UretprobeGnutlsRecordRecv,
		"uretprobe_pr_read":              programs.UretprobePrRead,
		"uretprobe_ssl_read":             programs.UretprobeSslRead,
	}
	prog, ok := programsByName[name]
	return prog, ok
}

func findFirstExistingPath(paths ...string) (string, bool) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func parseGoTLSSymbols(binPath string) ([]string, error) {
	exe, err := elf.Open(binPath)
	if err != nil {
		return nil, err
	}
	defer exe.Close()

	symbols, err := exe.Symbols()
	if err != nil {
		symbols, err = exe.DynamicSymbols()
		if err != nil {
			return nil, err
		}
	}

	out := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, sym := range symbols {
		if name, ok := goTLSSymbolName(sym.Name); ok {
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no Go TLS symbols found in %s", binPath)
	}
	return out, nil
}

func goTLSSymbolName(name string) (string, bool) {
	switch name {
	case "crypto/tls.(*Conn).Write", "crypto/tls.(*Conn).Read":
		return name, true
	default:
		return "", false
	}
}

func tlsProgramForSymbol(symbol string) (string, bool) {
	switch symbol {
	case "SSL_write", "SSL_write_ex":
		return "uprobe_ssl_write", true
	case "gnutls_record_send":
		return "uprobe_gnutls_record_send", true
	case "PR_Write":
		return "uprobe_pr_write", true
	case "crypto/tls.(*Conn).Write":
		return "uprobe_crypto_tls_conn_write", true
	case "crypto/tls.(*Conn).Read":
		return "uprobe_crypto_tls_conn_read", true
	case "gnutls_record_recv":
		return "uprobe_gnutls_record_recv", true
	case "PR_Read":
		return "uprobe_pr_read", true
	case "SSL_read", "SSL_read_ex":
		return "uprobe_ssl_read", true
	default:
		return "", false
	}
}

func tlsReturnProgramForSymbol(symbol string) (string, bool) {
	switch symbol {
	case "SSL_read", "SSL_read_ex":
		return "uretprobe_ssl_read", true
	case "gnutls_record_recv":
		return "uretprobe_gnutls_record_recv", true
	case "PR_Read":
		return "uretprobe_pr_read", true
	case "crypto/tls.(*Conn).Read":
		return "uretprobe_crypto_tls_conn_read", true
	default:
		return "", false
	}
}

func tlsProgramForSymbolName(symbol string) string {
	if name, ok := tlsProgramForSymbol(symbol); ok {
		return name
	}
	return ""
}

func tlsReturnProgramForSymbolName(symbol string) string {
	if name, ok := tlsReturnProgramForSymbol(symbol); ok {
		return name
	}
	return ""
}

func parseProcPID(path string) (int, bool) {
	cleaned := filepath.Clean(path)
	parts := strings.Split(cleaned, string(os.PathSeparator))
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] != "proc" {
			continue
		}
		pid, err := strconv.Atoi(parts[i+1])
		if err != nil || pid <= 0 {
			return 0, false
		}
		return pid, true
	}
	return 0, false
}

func (m *TLSProbeManager) shouldAttachGoBinary(binPath string, pid int) bool {
	if m == nil {
		return false
	}
	key := goAttachKey(binPath, pid)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.attachedGo == nil {
		m.attachedGo = make(map[string]bool)
	}
	if m.attachedGo[key] {
		return false
	}
	m.attachedGo[key] = true
	return true
}

func (m *TLSProbeManager) forgetGoBinaryAttach(binPath string, pid int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.attachedGo, goAttachKey(binPath, pid))
}

func goAttachKey(binPath string, pid int) string {
	return fmt.Sprintf("%d\x00%s", pid, binPath)
}

func (m *TLSProbeManager) DiscoverGoProcesses() {
	if m == nil {
		return
	}
	entries, err := filepath.Glob("/proc/[0-9]*/exe")
	if err != nil {
		return
	}
	for _, exeLink := range entries {
		pid, ok := parseProcPID(exeLink)
		if !ok {
			continue
		}
		binPath, err := os.Readlink(exeLink)
		if err != nil || binPath == "" {
			continue
		}
		if !m.shouldAttachGoBinary(binPath, pid) {
			continue
		}
		if err := m.AttachGoUprobes(binPath, pid); err != nil {
			m.forgetGoBinaryAttach(binPath, pid)
			if m.store != nil {
				m.store.SetLibraryStatus(TLSLibraryStatus{Name: "Go", Path: binPath, Attached: false, Available: true, Error: err.Error()})
			}
		}
	}
}

func (m *TLSProbeManager) StartGoDiscoveryLoop(interval time.Duration) {
	if m == nil {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		m.DiscoverGoProcesses()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			m.mu.Lock()
			closed := m.closed
			m.mu.Unlock()
			if closed {
				return
			}
			m.DiscoverGoProcesses()
		}
	}()
}
