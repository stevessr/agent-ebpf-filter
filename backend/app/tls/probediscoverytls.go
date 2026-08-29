package tls

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

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

func goAttachKey(binPath string, pid int) string {
	return fmt.Sprintf("%d\x00%s", pid, binPath)
}

func staticSSLAttachKey(binPath string, pid int) string {
	return fmt.Sprintf("exec\x00%d\x00%s", pid, binPath)
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

func (m *TLSProbeManager) shouldAttachStaticSSL(binPath string, pid int) bool {
	if m == nil {
		return false
	}
	key := staticSSLAttachKey(binPath, pid)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.attachedStatic == nil {
		m.attachedStatic = make(map[string]bool)
	}
	if m.attachedStatic[key] {
		return false
	}
	m.attachedStatic[key] = true
	return true
}

func (m *TLSProbeManager) forgetGoBinaryAttach(binPath string, pid int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	delete(m.attachedGo, goAttachKey(binPath, pid))
	m.mu.Unlock()
}

func (m *TLSProbeManager) forgetStaticSSLAttach(binPath string, pid int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	delete(m.attachedStatic, staticSSLAttachKey(binPath, pid))
	m.mu.Unlock()
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

const procDeletedSuffix = " (deleted)"

// normalizeProcExecutableTarget keeps a stable display path while allowing
// discovery to attach to /proc/<pid>/exe when an in-place package upgrade has
// unlinked the executable that the still-running process is using.
func normalizeProcExecutableTarget(exeLink, linkTarget string, procLinkUsable bool) (attachPath, displayPath string, deleted, ok bool) {
	linkTarget = strings.TrimSpace(linkTarget)
	if linkTarget == "" {
		return "", "", false, false
	}
	if !strings.HasSuffix(linkTarget, procDeletedSuffix) {
		return linkTarget, linkTarget, false, true
	}
	displayPath = strings.TrimSuffix(linkTarget, procDeletedSuffix)
	if !procLinkUsable {
		return "", displayPath, true, false
	}
	return exeLink, displayPath, true, true
}

func resolveProcExecutableTarget(exeLink string) (attachPath, displayPath string, deleted, ok bool) {
	linkTarget, err := os.Readlink(exeLink)
	if err != nil || strings.TrimSpace(linkTarget) == "" {
		return "", "", false, false
	}
	usable := true
	if strings.HasSuffix(linkTarget, procDeletedSuffix) {
		_, err = os.Stat(exeLink)
		usable = err == nil
	}
	return normalizeProcExecutableTarget(exeLink, linkTarget, usable)
}

func pidFromGoAttachKey(key string) (int, bool) {
	pidText, _, ok := strings.Cut(key, "\x00")
	if !ok {
		return 0, false
	}
	pid, err := strconv.Atoi(pidText)
	return pid, err == nil && pid > 0
}

func pidFromStaticAttachKey(key string) (int, bool) {
	var rest string
	switch {
	case strings.HasPrefix(key, "exec\x00"):
		rest = strings.TrimPrefix(key, "exec\x00")
	case strings.HasPrefix(key, "pid\x00"):
		rest = strings.TrimPrefix(key, "pid\x00")
	default:
		return 0, false
	}
	pidText, _, ok := strings.Cut(rest, "\x00")
	if !ok {
		return 0, false
	}
	pid, err := strconv.Atoi(pidText)
	return pid, err == nil && pid > 0
}

func (m *TLSProbeManager) pruneDeadProcessAttachments() {
	if m == nil {
		return
	}
	m.mu.Lock()
	for pid := range m.attachedExec {
		if !processExists(pid) {
			delete(m.attachedExec, pid)
		}
	}
	for key := range m.attachedGo {
		if pid, ok := pidFromGoAttachKey(key); ok && !processExists(pid) {
			delete(m.attachedGo, key)
		}
	}
	for key := range m.attachedStatic {
		if pid, ok := pidFromStaticAttachKey(key); ok && !processExists(pid) {
			delete(m.attachedStatic, key)
		}
	}
	m.mu.Unlock()
	m.pruneAutoDiscoveryState()
}

func (m *TLSProbeManager) pidAlreadyAttached(pid int) bool {
	if m == nil || pid <= 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.attachedExec[pid]; ok {
		return true
	}
	for key := range m.attachedGo {
		attachedPID, ok := pidFromGoAttachKey(key)
		if ok && attachedPID == pid {
			return true
		}
	}
	return false
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
		if !ok || m.pidAlreadyAttached(pid) {
			continue
		}
		attachPath, displayPath, _, ok := resolveProcExecutableTarget(exeLink)
		if !ok {
			continue
		}

		if _, err := parseGoTLSTargets(attachPath); err != nil {
			continue
		}
		now := time.Now()
		if !m.autoAttachAllowed("go", pid, attachPath, now) {
			continue
		}
		if !m.shouldAttachGoBinary(attachPath, pid) {
			continue
		}
		m.recordAutoAttachAttempt()
		if err := m.AttachGoUprobes(attachPath, pid); err != nil {
			m.forgetGoBinaryAttach(attachPath, pid)
			m.recordAutoAttachFailure("go", pid, attachPath, err, now)
			if m.store != nil {
				m.store.SetLibraryStatus(TLSLibraryStatus{Name: "Go", Path: displayPath, Attached: false, Available: true, Error: err.Error()})
			}
			continue
		}
		m.recordAutoAttachSuccess("go", pid, attachPath)
	}
}

func normalizedProcCmdline(pid int) string {
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(string(cmdline), "\x00", " ")))
}

func isAgentTLSProcess(baseName, cmdline string) bool {
	base := strings.ToLower(strings.TrimSpace(baseName))
	cmd := strings.ToLower(cmdline)

	for _, direct := range []string{
		"claude", "codex", "opencode", "aider", "goose", "cursor", "amp",
		"gemini", "dsh", "omp", "cline", "windsurf",
	} {
		if base == direct || strings.HasPrefix(base, direct+"-") {
			return true
		}
	}

	for _, marker := range []string{
		"claude-code", "@anthropic", "@cometix", "anthropic",
		"codex", "@openai", "openai",
		"opencode", "oh-my-pi", "oh_my_pi", "aider", "goose",
		"cursor", "github-copilot", "copilot", "gemini", "continue",
		"cline", "windsurf", "qwen-code", "kimi-cli", "roo-code",
		" dsh ", " omp ",
	} {
		if strings.Contains(cmd, marker) {
			return true
		}
	}
	return false
}

// DiscoverNodeProcesses keeps the historical name for API compatibility but
// now discovers agent runtimes broadly and lets AttachExecutable choose Go,
// static OpenSSL/BoringSSL, rustls, or an actually loaded shared library.
func (m *TLSProbeManager) DiscoverNodeProcesses() {
	if m == nil {
		return
	}
	entries, err := filepath.Glob("/proc/[0-9]*/exe")
	if err != nil {
		return
	}
	for _, exeLink := range entries {
		pid, ok := parseProcPID(exeLink)
		if !ok || m.pidAlreadyAttached(pid) {
			continue
		}
		attachPath, displayPath, _, ok := resolveProcExecutableTarget(exeLink)
		if !ok {
			continue
		}

		baseName := filepath.Base(displayPath)
		if !isAgentTLSProcess(baseName, normalizedProcCmdline(pid)) {
			continue
		}
		now := time.Now()
		if !m.autoAttachAllowed("agent", pid, attachPath, now) {
			continue
		}
		if !m.shouldAttachStaticSSL(attachPath, pid) {
			continue
		}

		m.recordAutoAttachAttempt()
		result := m.AttachExecutable(attachPath, pid, "auto")
		if result.Error != "" {
			m.forgetStaticSSLAttach(attachPath, pid)
			attachErr := fmt.Errorf("%s", result.Error)
			m.recordAutoAttachFailure("agent", pid, attachPath, attachErr, now)
			if m.store != nil {
				m.store.SetLibraryStatus(TLSLibraryStatus{Name: "auto:" + baseName, Path: displayPath, Attached: false, Available: true, Error: result.Error})
			}
			continue
		}
		m.recordAutoAttachSuccess("agent", pid, attachPath)
		if m.store != nil {
			library := result.Library
			if library == "" {
				library = result.TargetKind
			}
			m.store.SetLibraryStatus(TLSLibraryStatus{Name: "auto:" + library, Path: displayPath, Attached: true, Available: true})
		}
	}
}

func (m *TLSProbeManager) StartGoDiscoveryLoop(interval time.Duration) {
	m.startGoDiscoveryLoop(interval, func() {
		m.pruneDeadProcessAttachments()
		m.DiscoverGoProcesses()
		m.DiscoverNodeProcesses()
	})
}

func (m *TLSProbeManager) startGoDiscoveryLoop(interval time.Duration, discover func()) {
	if m == nil || discover == nil {
		return
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	m.mu.Lock()
	if m.closed || m.discoveryStarted {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.discoveryStarted = true
	m.discoveryCancel = cancel
	m.discoveryWG.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.discoveryWG.Done()
		if ctx.Err() != nil {
			return
		}
		discover()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				discover()
			}
		}
	}()
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
	case "SSL_write":
		return "uprobe_ssl_write", true
	case "SSL_write_ex":
		return "uprobe_ssl_write_ex", true
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
	case "SSL_read":
		return "uprobe_ssl_read", true
	case "SSL_read_ex":
		return "uprobe_ssl_read_ex", true
	case "SSL_write_ex2":
		return "uprobe_ssl_write_ex2", true
	default:
		return "", false
	}
}

func tlsReturnProgramForSymbol(symbol string) (string, bool) {
	switch symbol {
	case "SSL_read":
		return "uretprobe_ssl_read", true
	case "SSL_read_ex":
		return "uretprobe_ssl_read_ex", true
	case "SSL_write_ex":
		return "uretprobe_ssl_write_ex", true
	case "gnutls_record_recv":
		return "uretprobe_gnutls_record_recv", true
	case "PR_Read":
		return "uretprobe_pr_read", true
	case "crypto/tls.(*Conn).Read":
		return "uretprobe_crypto_tls_conn_read", true
	case "SSL_write_ex2":
		return "uretprobe_ssl_write_ex2", true
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
