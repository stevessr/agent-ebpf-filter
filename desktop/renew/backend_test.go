package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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
	t.Setenv("AGENT_RENEW_BACKEND_BIN", "/missing/backend")
	session, err := ensureBackend(context.Background(), server.URL, "unused")
	if err != nil || session != nil {
		t.Fatalf("existing authenticated backend not reused: %v", err)
	}
}

func TestLocalBackendAutoStartBoundary(t *testing.T) {
	for _, origin := range []string{"https://example.com", "http://192.0.2.1:8080", "http://localhost:8080/base"} {
		if _, err := localBackendPort(origin); err == nil {
			t.Fatalf("auto-start allowed for %s", origin)
		}
	}
	for _, origin := range []string{"http://127.0.0.1:8080", "http://[::1]:8080"} {
		if _, err := localBackendPort(origin); err != nil {
			t.Fatalf("local backend rejected for %s: %v", origin, err)
		}
	}
}

func TestBundledBackendStartupAndCleanup(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 needed for fake backend")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENT_RENEW_BACKEND_BIN", "")
	t.Setenv("AGENT_RENEW_FRONTEND_DIST", "")
	t.Setenv("AGENT_RENEW_DEV", "")
	resources := t.TempDir()
	if err := os.Mkdir(filepath.Join(resources, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(resources, "backend", "agent-ebpf-filter")
	// Fixture models the private token handshake, API readiness and EOF shutdown;
	// it does not claim to exercise pkexec or the real kernel.
	source := `#!/usr/bin/env python3
import argparse, socket, json, threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
p = argparse.ArgumentParser()
p.add_argument('--desktop-lifetime-socket'); p.add_argument('--real-home')
p.add_argument('--desktop-port', type=int); p.add_argument('--frontend-dir')
a = p.parse_args()
c = socket.socket(socket.AF_UNIX); c.connect(a.desktop_lifetime_socket)
c.sendall(json.dumps({'token':'fixture-token'}).encode()+b'\n')
class Handler(BaseHTTPRequestHandler):
 def do_GET(self):
  self.send_response(200); self.send_header('Content-Type','application/json'); self.end_headers(); self.wfile.write(b'[]')
 def log_message(self, *args): pass
s = ThreadingHTTPServer(('127.0.0.1', a.desktop_port), Handler)
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
	session, err := ensureBackend(ctx, origin, resources)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.token != "fixture-token" {
		t.Fatal("private auth handshake failed")
	}
	dir := session.dir
	session.Close()
	if backendAPIAvailable(ctx, origin) {
		t.Fatal("owned backend still running after desktop close")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("session directory was not cleaned")
	}
}

func TestBackendAuthorizationFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	binary := filepath.Join(t.TempDir(), "denied")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 126\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_RENEW_BACKEND_BIN", binary)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	session, err := ensureBackend(ctx, "http://127.0.0.1:1", t.TempDir())
	if session != nil || err == nil || !strings.Contains(err.Error(), "authorization/startup failed") {
		t.Fatalf("missing authorization failure: %v", err)
	}
}

func TestGraphicalAskpassSelection(t *testing.T) {
	resources := t.TempDir()
	tools := t.TempDir()
	t.Setenv("PATH", tools)
	for _, name := range []string{"sudo", "zenity"} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if graphicalAskpass(resources) != "" {
		t.Fatal("selected missing bundled helper")
	}
	helper := filepath.Join(resources, "renew-askpass.sh")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if graphicalAskpass(resources) != helper {
		t.Fatal("GUI askpass not selected without a polkit agent")
	}
}
