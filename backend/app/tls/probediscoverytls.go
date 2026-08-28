package tls

import (
	"context"
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type goTLSSymbolCacheEntry struct {
	size        int64
	modUnixNano int64
	symbols     []string
	err         string
}

var goTLSSymbolCache sync.Map

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

func staticSSLAttachKey(binPath string, pid int) string {
	return fmt.Sprintf("exec\x00%d\x00%s", pid, binPath)
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
	defer m.mu.Unlock()
	delete(m.attachedGo, goAttachKey(binPath, pid))
}

func (m *TLSProbeManager) forgetStaticSSLAttach(binPath string, pid int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.attachedStatic, staticSSLAttachKey(binPath, pid))
}

func goAttachKey(binPath string, pid int) string {
	return fmt.Sprintf("%d\x00%s", pid, binPath)
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
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
	if !strings.HasPrefix(key, "exec\x00") {
		return 0, false
	}
	rest := strings.TrimPrefix(key, "exec\x00")
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
	defer m.mu.Unlock()
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
		binPath, err := os.Readlink(exeLink)
		if err != nil || binPath == "" || strings.HasSuffix(binPath, " (deleted)") {
			continue
		}

		// Parse/cached-classify the binary before reserving an attach key. This
		// avoids repeatedly marking every non-Go process as a failed Go attach on
		// each /proc discovery pass.
		if _, err := parseGoTLSSymbols(binPath); err != nil {
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

// DiscoverNodeProcesses keeps the historical name for API/test compatibility,
// but now discovers agent runtimes broadly (Node/Bun/Deno, Python, Rust-native
// CLIs, etc.) and lets AttachExecutable select the best TLS strategy.
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
		binPath, err := os.Readlink(exeLink)
		if err != nil || binPath == "" || strings.HasSuffix(binPath, " (deleted)") {
			continue
		}

		baseName := filepath.Base(binPath)
		cmdline := normalizedProcCmdline(pid)
		if !isAgentTLSProcess(baseName, cmdline) {
			continue
		}
		if !m.shouldAttachStaticSSL(binPath, pid) {
			continue
		}

		result := m.AttachExecutable(binPath, pid, "auto")
		if result.Error != "" {
			m.forgetStaticSSLAttach(binPath, pid)
			if m.store != nil {
				m.store.SetLibraryStatus(TLSLibraryStatus{
					Name:      "auto:" + baseName,
					Path:      binPath,
					Attached:  false,
					Available: true,
					Error:     result.Error,
				})
			}
			continue
		}

		if m.store != nil {
			library := result.Library
			if library == "" {
				library = result.TargetKind
			}
			m.store.SetLibraryStatus(TLSLibraryStatus{
				Name:      "auto:" + library,
				Path:      binPath,
				Attached:  true,
				Available: true,
			})
		}
	}
}

func hasSSLSymbols(binPath string) bool {
	exe, err := elf.Open(binPath)
	if err != nil {
		return false
	}
	defer exe.Close()

	symbols, err := exe.Symbols()
	if err != nil {
		symbols, _ = exe.DynamicSymbols()
	}

	for _, sym := range symbols {
		if sym.Name == "SSL_write" || sym.Name == "SSL_read" || sym.Name == "SSL_write_ex" || sym.Name == "SSL_read_ex" || sym.Name == "SSL_write_ex2" {
			return true
		}
	}
	return false
}

func (m *TLSProbeManager) StartGoDiscoveryLoop(interval time.Duration) {
	m.startGoDiscoveryLoop(interval, func() {
		m.pruneDeadProcessAttachments()
		m.DiscoverGoProcesses()
		m.DiscoverNodeProcesses()
	})
}

func (m *TLSProbeManager) startGoDiscoveryLoop(interval time.Duration, discover func()) {
	if m == nil {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	if discover == nil {
		return
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

func parseGoTLSSymbols(binPath string) ([]string, error) {
	info, statErr := os.Stat(binPath)
	if statErr != nil {
		return nil, statErr
	}
	cacheKey := filepath.Clean(binPath)
	if cached, ok := goTLSSymbolCache.Load(cacheKey); ok {
		entry := cached.(goTLSSymbolCacheEntry)
		if entry.size == info.Size() && entry.modUnixNano == info.ModTime().UnixNano() {
			if entry.err != "" {
				return nil, fmt.Errorf("%s", entry.err)
			}
			return append([]string(nil), entry.symbols...), nil
		}
	}

	exe, err := elf.Open(binPath)
	if err != nil {
		goTLSSymbolCache.Store(cacheKey, goTLSSymbolCacheEntry{size: info.Size(), modUnixNano: info.ModTime().UnixNano(), err: err.Error()})
		return nil, err
	}
	defer exe.Close()

	symbols, err := exe.Symbols()
	if err != nil {
		symbols, err = exe.DynamicSymbols()
		if err != nil {
			cacheErr := fmt.Errorf("no ELF symbols available in %s: %w", binPath, err)
			goTLSSymbolCache.Store(cacheKey, goTLSSymbolCacheEntry{size: info.Size(), modUnixNano: info.ModTime().UnixNano(), err: cacheErr.Error()})
			return nil, cacheErr
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
		cacheErr := fmt.Errorf("no Go TLS symbols found in %s", binPath)
		goTLSSymbolCache.Store(cacheKey, goTLSSymbolCacheEntry{size: info.Size(), modUnixNano: info.ModTime().UnixNano(), err: cacheErr.Error()})
		return nil, cacheErr
	}
	goTLSSymbolCache.Store(cacheKey, goTLSSymbolCacheEntry{size: info.Size(), modUnixNano: info.ModTime().UnixNano(), symbols: append([]string(nil), out...)})
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
