package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestReuseBackendWithoutLaunching(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events/summaries" {
			t.Error("wrong readiness route")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Unauthorized"}`))
	}))
	defer server.Close()
	session, err := ensureBackend(context.Background(), server.URL)
	if err != nil || session != nil {
		t.Fatalf("existing authenticated backend not reused: %v", err)
	}
}

func TestLocalOriginAndTokenIsolation(t *testing.T) {
	for _, origin := range []string{"https://example.com", "http://192.0.2.1:8080", "http://localhost:8080/base"} {
		if _, err := localBackendPort(origin); err == nil {
			t.Fatalf("auto-start allowed for %s", origin)
		}
		if tokenPreload(origin, "secret") != "" {
			t.Fatal("local token exposed to a nonlocal/custom origin")
		}
	}
	script := tokenPreload("http://127.0.0.1:8080", `quote"test`)
	if !strings.Contains(script, "location.origin ===") || !strings.Contains(script, "agent-ebpf.apiToken") {
		t.Fatal("missing origin-scoped auth preload")
	}
}

func TestBundledBackendStartupAndCleanup(t *testing.T) {
	if runtime.GOOS != "linux" { t.Skip("Linux-only backend / filesystem test") }
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 needed for fake backend")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENT_RENEW_DEV", "")
	binary := filepath.Join(t.TempDir(), "renew-fixture")
	oldExecutable := renewExecutable
	renewExecutable = func() (string, error) { return binary, nil }
	defer func() { renewExecutable = oldExecutable }()
	// Fixture models the same-binary internal backend dispatch, private token
	// handshake, API readiness and EOF shutdown. It does not exercise pkexec
	// or the real kernel.
	source := `#!/usr/bin/env python3
import argparse, socket, json, threading
from http.server import BaseHTTPRequestHandler
from socketserver import UnixStreamServer
p = argparse.ArgumentParser()
p.add_argument('--internal-backend', action='store_true')
p.add_argument('--desktop-lifetime-socket'); p.add_argument('--real-home')
p.add_argument('--desktop-api-socket'); p.add_argument('--frontend-dir')
a = p.parse_args()
c = socket.socket(socket.AF_UNIX); c.connect(a.desktop_lifetime_socket)
c.sendall(json.dumps({'token':'fixture-token'}).encode()+b'\n')
class Handler(BaseHTTPRequestHandler):
 def do_GET(self):
  self.send_response(200); self.send_header('Content-Type','application/json'); self.end_headers(); self.wfile.write(b'[]')
 def log_message(self, *args): pass
s = UnixStreamServer(a.desktop_api_socket, Handler)
threading.Thread(target=s.serve_forever, daemon=True).start()
c.recv(1)
s.shutdown(); s.server_close(); c.close()
`
	if err := os.WriteFile(binary, []byte(source), 0755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	_ = listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := ensureBackend(ctx, origin)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.token != "fixture-token" {
		t.Fatal("private auth handshake failed")
	}
	if session.apiSocket == "" || session.clientAddress() == "" {
		t.Fatal("expected owner-private Unix API socket")
	}
	dir := session.dir
	session.Close()
	if backendUnixAPIAvailable(ctx, filepath.Join(dir, "api.sock"), "fixture-token") {
		t.Fatal("owned Unix backend still running after desktop close")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("session directory was not cleaned")
	}
}

func TestBackendAuthorizationFailure(t *testing.T) {
	if runtime.GOOS != "linux" { t.Skip("Linux-only backend / filesystem test") }
	t.Setenv("HOME", t.TempDir())
	binary := filepath.Join(t.TempDir(), "denied-renew")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 126\n"), 0755); err != nil {
		t.Fatal(err)
	}
	oldExecutable := renewExecutable
	renewExecutable = func() (string, error) { return binary, nil }
	defer func() { renewExecutable = oldExecutable }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	session, err := ensureBackend(ctx, "http://127.0.0.1:1")
	if session != nil || err == nil || !strings.Contains(err.Error(), "authorization/startup failed") {
		t.Fatalf("missing authorization failure: %v", err)
	}
}

func TestSelfAskpassSelection(t *testing.T) {
	if runtime.GOOS != "linux" { t.Skip("Linux-only backend / filesystem test") }
	tools := t.TempDir()
	t.Setenv("PATH", tools)
	t.Setenv("SUDO_ASKPASS", "")
	for _, name := range []string{"sudo", "zenity"} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.Command("/bin/true")
	cmd.Env = []string{"PATH=" + tools}
	binary := "/opt/renew/renew"
	configureBackendAskpass(cmd, binary)

	joined := strings.Join(cmd.Env, "\n")
	if !strings.Contains(joined, "SUDO_ASKPASS="+binary) {
		t.Fatalf("self askpass path missing from env: %s", joined)
	}
	if !strings.Contains(joined, askpassModeEnv+"=1") {
		t.Fatalf("self askpass mode missing from env: %s", joined)
	}
}

func TestUnixClientDoesNotRequireTCP(t *testing.T) {
	if runtime.GOOS != "linux" { t.Skip("Linux-only backend / filesystem test") }
	dir := t.TempDir()
	socket := filepath.Join(dir, "api.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil { t.Fatal(err) }
	defer listener.Close()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events/summaries" || r.Header.Get("X-API-KEY") != "local-only" {
			t.Errorf("unexpected Unix API request: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[]}`))
	})}
	go server.Serve(listener)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !backendUnixAPIAvailable(ctx, socket, "local-only") {
		t.Fatal("authenticated API over Unix socket did not respond")
	}
}
