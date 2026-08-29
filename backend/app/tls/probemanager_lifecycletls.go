package tls

import (
	"errors"
	"fmt"
	"os"
	"strings"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
)

func tlsFuncName(fn uint8) string {
	switch fn {
	case tlsFuncSSLWrite:
		return "SSL_write"
	case tlsFuncSSLRead:
		return "SSL_read"
	case tlsFuncSSLWriteEx:
		return "SSL_write_ex"
	case tlsFuncSSLReadEx:
		return "SSL_read_ex"
	case tlsFuncGnuTLSRecordSend:
		return "gnutls_record_send"
	case tlsFuncGnuTLSRecordRecv:
		return "gnutls_record_recv"
	case tlsFuncPRWrite:
		return "PR_Write"
	case tlsFuncPRRead:
		return "PR_Read"
	case tlsFuncGoConnWrite:
		return "crypto/tls.Write"
	case tlsFuncGoConnRead:
		return "crypto/tls.Read"
	case tlsFuncSSLWriteEx2:
		return "SSL_write_ex2"
	case tlsFuncRustlsEncryptOutgoing:
		return "rustls::encrypt_outgoing"
	case tlsFuncRustlsConsumeFirstChunk:
		return "rustls::consume_first_chunk"
	default:
		return "unknown"
	}
}

func (m *TLSProbeManager) Close() error {
	defer dropDiscoveryRuntimeState(m)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	discoveryCancel := m.discoveryCancel
	m.discoveryCancel = nil
	reader := m.reader
	m.reader = nil
	m.mu.Unlock()

	if discoveryCancel != nil {
		discoveryCancel()
	}
	if reader != nil {
		_ = reader.Close()
	}
	m.discoveryWG.Wait()

	m.mu.Lock()
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
		"uprobe_crypto_tls_conn_read":       programs.UprobeCryptoTlsConnRead,
		"uprobe_crypto_tls_conn_write":      programs.UprobeCryptoTlsConnWrite,
		"uprobe_gnutls_record_recv":         programs.UprobeGnutlsRecordRecv,
		"uprobe_gnutls_record_send":         programs.UprobeGnutlsRecordSend,
		"uprobe_pr_read":                    programs.UprobePrRead,
		"uprobe_pr_write":                   programs.UprobePrWrite,
		"uprobe_ssl_read":                   programs.UprobeSslRead,
		"uprobe_ssl_read_ex":                programs.UprobeSslReadEx,
		"uprobe_ssl_write":                  programs.UprobeSslWrite,
		"uprobe_ssl_write_ex":               programs.UprobeSslWriteEx,
		"uprobe_ssl_write_ex2":              programs.UprobeSslWriteEx2,
		"uprobe_rustls_encrypt_outgoing":    programs.UprobeRustlsEncryptOutgoing,
		"uprobe_rustls_consume_first_chunk": programs.UprobeRustlsConsumeFirstChunk,
		"uretprobe_ssl_write_ex2":           programs.UretprobeSslWriteEx2,
		"uretprobe_crypto_tls_conn_read":    programs.UretprobeCryptoTlsConnRead,
		"uretprobe_gnutls_record_recv":      programs.UretprobeGnutlsRecordRecv,
		"uretprobe_pr_read":                 programs.UretprobePrRead,
		"uretprobe_ssl_read":                programs.UretprobeSslRead,
		"uretprobe_ssl_read_ex":             programs.UretprobeSslReadEx,
		"uretprobe_ssl_write_ex":            programs.UretprobeSslWriteEx,
	}
	prog, ok := programsByName[name]
	return prog, ok
}

// AttachedPIDInfo describes a process that has an active SSL/TLS uprobe.
type AttachedPIDInfo struct {
	PID               int    `json:"pid"`
	BinaryPath        string `json:"binary_path"`
	LibraryName       string `json:"library_name"`
	CaptureObserved   bool   `json:"capture_observed"`
	LastCaptureUnixMs int64  `json:"last_capture_unix_ms,omitempty"`
}

func (m *TLSProbeManager) attachObservation(pid int) (bool, int64) {
	lastNS, observed := m.tlsCaptureObservation(pid)
	if !observed {
		return false, 0
	}
	return true, lastNS / 1_000_000
}

// AttachedPIDs returns the list of PIDs that currently have SSL/TLS uprobes
// attached. capture_observed distinguishes "attach syscall succeeded" from a
// probe which has actually emitted at least one TLS fragment for that process.
func (m *TLSProbeManager) AttachedPIDs() []AttachedPIDInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	seen := make(map[int]bool)
	var result []AttachedPIDInfo

	for key := range m.attachedGo {
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		pid := 0
		fmt.Sscanf(parts[0], "%d", &pid)
		if pid > 0 && !seen[pid] {
			seen[pid] = true
			observed, lastCaptureMs := m.attachObservation(pid)
			result = append(result, AttachedPIDInfo{
				PID:               pid,
				BinaryPath:        parts[1],
				LibraryName:       "go-crypto-tls",
				CaptureObserved:   observed,
				LastCaptureUnixMs: lastCaptureMs,
			})
		}
	}

	for pid, lib := range m.attachedExec {
		if pid > 0 && !seen[pid] {
			seen[pid] = true
			binPath := ""
			if executablePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
				binPath = executablePath
			}
			observed, lastCaptureMs := m.attachObservation(pid)
			result = append(result, AttachedPIDInfo{
				PID:               pid,
				BinaryPath:        binPath,
				LibraryName:       lib,
				CaptureObserved:   observed,
				LastCaptureUnixMs: lastCaptureMs,
			})
		}
	}

	return result
}

func (m *TLSProbeManager) ReadLoopStatsSnapshot() ReadLoopStats {
	if m == nil {
		return ReadLoopStats{}
	}
	return m.readLoopStats.Snapshot()
}

func (m *TLSProbeManager) ProbeHitCounters() map[string]uint64 {
	result := make(map[string]uint64)
	if m == nil || m.objs == nil || m.objs.TlsProbeHits == nil {
		return result
	}
	for fn := uint8(1); fn <= 13; fn++ {
		var idx uint32 = uint32(fn)
		var val uint64
		if err := m.objs.TlsProbeHits.Lookup(&idx, &val); err == nil && val > 0 {
			result[tlsFuncName(fn)] = val
		}
	}
	diagLabels := map[uint32]string{100: "perf_output_fail", 101: "probe_read_fail", 102: "perf_submit_ok"}
	for idx, label := range diagLabels {
		var val uint64
		if err := m.objs.TlsProbeHits.Lookup(&idx, &val); err == nil && val > 0 {
			result[label] = val
		}
	}
	return result
}
