package tls

import (
	"debug/elf"
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
		if !ok {
			continue
		}
		binPath, err := os.Readlink(exeLink)
		if err != nil || binPath == "" {
			continue
		}

		baseName := filepath.Base(binPath)
		if baseName != "node" && baseName != "bun" && baseName != "deno" && baseName != "codex" {
			continue
		}

		if !m.shouldAttachStaticSSL(binPath, pid) {
			continue
		}

		cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		cmdStr := string(cmdline)

		isClaudeCode := strings.Contains(cmdStr, "claude-code") || strings.Contains(cmdStr, "@cometix")
		isCodex := strings.Contains(cmdStr, "codex") || strings.Contains(cmdStr, "@openai") || baseName == "codex"

		if !isClaudeCode && !isCodex {
			m.forgetGoBinaryAttach(binPath, pid)
			continue
		}

		// Node.js/Bun/Deno: 使用 OpenSSL 符号
		if baseName == "node" || baseName == "bun" || baseName == "deno" {
			if hasSSLSymbols(binPath) {
				if err := m.AttachStaticSSLUprobes(binPath, pid); err != nil {
					m.forgetGoBinaryAttach(binPath, pid)
					if m.store != nil {
						name := "Node.js"
						if isClaudeCode {
							name = "Claude Code (Node.js)"
						}
						m.store.SetLibraryStatus(TLSLibraryStatus{Name: name, Path: binPath, Attached: false, Available: true, Error: err.Error()})
					}
				}
			}
			continue
		}

		// Codex: Rust 二进制，使用 rustls 偏移量
		if isCodex {
			if err := m.AttachRustlsUprobes(binPath, pid); err != nil {
				m.forgetGoBinaryAttach(binPath, pid)
				if m.store != nil {
					m.store.SetLibraryStatus(TLSLibraryStatus{Name: "Codex (rustls)", Path: binPath, Attached: false, Available: true, Error: err.Error()})
				}
			}
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
		if sym.Name == "SSL_write" || sym.Name == "SSL_read" || sym.Name == "SSL_write_ex" || sym.Name == "SSL_read_ex" {
			return true
		}
	}
	return false
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
		m.DiscoverNodeProcesses()
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
			m.DiscoverNodeProcesses()
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
