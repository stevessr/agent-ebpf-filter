package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type backendSession struct {
	dir      string
	listener net.Listener
	conn     net.Conn
	mu       sync.Mutex
	closed   bool
	token    string
	cmd      *exec.Cmd
	done     chan struct{}
}

func (s *backendSession) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	if s.conn != nil {
		_ = s.conn.Close()
	} else if s.cmd != nil {
		// Cancel an unprivileged launcher still awaiting authorization. Once
		// connected, the privileged child exits via socket EOF instead.
		cancelBackendLauncher(s.cmd)
	}
	_ = s.listener.Close()
	s.mu.Unlock()
	if s.done != nil {
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
		}
	}
	_ = os.RemoveAll(s.dir)
}

func backendAPIAvailable(ctx context.Context, origin string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/events/summaries?limit=1", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 401 && response.StatusCode != 403 {
		return false
	}
	if !strings.Contains(response.Header.Get("Content-Type"), "application/json") {
		return false
	}
	var body json.RawMessage
	return json.NewDecoder(response.Body).Decode(&body) == nil
}

func localBackendPort(origin string) (string, error) {
	parsed, err := url.Parse(origin)
	if err != nil {
		return "", err
	}
	host := parsed.Hostname()
	if parsed.Scheme != "http" || (host != "localhost" && host != "127.0.0.1" && host != "::1") || parsed.Path != "" {
		return "", fmt.Errorf("cannot auto-start a backend for %s; start that remote/custom instance separately", origin)
	}
	port := parsed.Port()
	if port == "" {
		port = "80"
	}
	return port, nil
}

// The desktop remains unprivileged. The backend itself requests pkexec (or
// sudo -A when SUDO_ASKPASS is configured); only that child gains privileges.
func ensureBackend(ctx context.Context, origin, resources string) (*backendSession, error) {
	if backendAPIAvailable(ctx, origin) {
		return nil, nil
	}
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("automatic eBPF backend startup requires Linux")
	}
	port, err := localBackendPort(origin)
	if err != nil {
		return nil, err
	}
	binary := os.Getenv("AGENT_RENEW_BACKEND_BIN")
	if binary == "" {
		binary = filepath.Join(resources, "backend", "agent-ebpf-filter")
	}
	if info, err := os.Stat(binary); err != nil || info.IsDir() {
		return nil, fmt.Errorf("bundled backend missing; build with make renew-desktop-build (or use make renew-desktop-dev)")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "renew-session-") // mode 0700: token channel is user-private.
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", filepath.Join(dir, "lifetime.sock"))
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	session := &backendSession{dir: dir, listener: listener}
	success := false
	defer func() {
		if !success {
			session.Close()
		}
	}()
	args := []string{"--desktop-lifetime-socket", filepath.Join(dir, "lifetime.sock"), "--real-home", home, "--desktop-port", port}
	if os.Getenv("AGENT_RENEW_DEV") == "true" {
		args = append(args, "--desktop-dev")
	}
	logDir := filepath.Join(home, ".config", "agent-ebpf-filter", "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, "renew-backend.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()
	cmd := exec.Command(binary, args...)
	configureBackendLauncher(cmd)
	cmd.Dir = dir // never write .port/logs into a read-only installed bundle.
	cmd.Env = os.Environ()
	// Some desktops run polkitd but no graphical authentication agent.
	// Prefer the bundled GUI askpass when a supported dialog tool exists.
	if os.Getenv("SUDO_ASKPASS") == "" {
		if helper := graphicalAskpass(resources); helper != "" {
			cmd.Env = append(cmd.Env, "SUDO_ASKPASS="+helper)
		}
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start backend: %w", err)
	}
	session.cmd = cmd
	session.done = make(chan struct{})
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait(); close(session.done) }()
	handshake := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			handshake <- err
			return
		}
		session.mu.Lock()
		if session.closed {
			_ = conn.Close()
			session.mu.Unlock()
			return
		}
		session.conn = conn
		session.mu.Unlock()
		var message struct {
			Token string `json:"token"`
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
		err = json.NewDecoder(conn).Decode(&message)
		if err == nil {
			session.token = message.Token
		}
		handshake <- err
	}()
	select {
	case err := <-handshake:
		if err != nil {
			return nil, fmt.Errorf("backend handshake: %w (log: %s)", err, logPath)
		}
	case err := <-exited:
		return nil, fmt.Errorf("backend authorization/startup failed: %v (log: %s)", err, logPath)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if backendAPIAvailable(ctx, origin) {
			success = true
			return session, nil
		}
		select {
		case err := <-exited:
			return nil, fmt.Errorf("backend exited: %v (log: %s)", err, logPath)
		case <-ctx.Done():
			return nil, fmt.Errorf("backend not ready: %w (log: %s)", ctx.Err(), logPath)
		case <-ticker.C:
		}
	}
}

func graphicalAskpass(resources string) string {
	helper := filepath.Join(resources, "renew-askpass.sh")
	if info, err := os.Stat(helper); err != nil || info.Mode()&0111 == 0 {
		return ""
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return ""
	}
	for _, dialog := range []string{"zenity", "kdialog"} {
		if _, err := exec.LookPath(dialog); err == nil {
			return helper
		}
	}
	return ""
}

func existingLocalToken(origin string) string {
	if _, err := localBackendPort(origin); err != nil {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "agent-ebpf-filter", "runtime.json"))
	if err != nil {
		return ""
	}
	var settings struct {
		AccessToken string `json:"accessToken"`
	}
	if json.Unmarshal(data, &settings) != nil {
		return ""
	}
	return settings.AccessToken
}
