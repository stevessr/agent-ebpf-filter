package main

import (
	"os"
	"path/filepath"
	"testing"
)

func envMap(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestResolveBackendURLPrecedence(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backend", ".port"), []byte("18080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "frontend", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		flag string
		env  map[string]string
		cwd  string
		want string
	}{
		{name: "flag wins", flag: "ws://10.0.0.5:9000/", env: map[string]string{"AGENT_BACKEND_URL": "http://ignored"}, want: "http://10.0.0.5:9000"},
		{name: "flag without scheme", flag: "localhost:8081", want: "http://localhost:8081"},
		{name: "env url", env: map[string]string{"AGENT_BACKEND_URL": "https://edge.example:8443/api/"}, want: "https://edge.example:8443/api"},
		{name: "env port", env: map[string]string{"AGENT_BACKEND_PORT": "9090"}, want: "http://127.0.0.1:9090"},
		{name: "port file from nested cwd", cwd: nested, want: "http://127.0.0.1:18080"},
		{name: "default", cwd: t.TempDir(), want: "http://127.0.0.1:8080"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveBackendURL(tc.flag, envMap(tc.env), tc.cwd)
			if err != nil {
				t.Fatalf("resolveBackendURL error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("resolveBackendURL = %q, want %q", got, tc.want)
			}
		})
	}
	if _, err := resolveBackendURL("ftp://x", envMap(nil), ""); err == nil {
		t.Fatal("unsupported scheme accepted")
	}
}

func TestResolveTokenPrecedence(t *testing.T) {
	home := t.TempDir()
	settingsDir := filepath.Join(home, ".config", "agent-ebpf-filter")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "runtime.json"), []byte(`{"accessToken":" file-token ","other":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := resolveToken("flag-token", envMap(map[string]string{"AGENT_ACCESS_TOKEN": "env"}), home); got != "flag-token" {
		t.Fatalf("flag token = %q", got)
	}
	if got := resolveToken("", envMap(map[string]string{"AGENT_ACCESS_TOKEN": "env-token"}), home); got != "env-token" {
		t.Fatalf("env token = %q", got)
	}
	if got := resolveToken("", envMap(nil), home); got != "file-token" {
		t.Fatalf("file token = %q", got)
	}
	if got := resolveToken("", envMap(nil), t.TempDir()); got != "" {
		t.Fatalf("missing file token = %q, want empty", got)
	}
}

func TestConfigValidate(t *testing.T) {
	if err := (Config{BackendURL: "http://x", History: 10}).validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if err := (Config{History: 10}).validate(); err == nil {
		t.Fatal("missing backend accepted")
	}
	if err := (Config{BackendURL: "http://x", History: 0}).validate(); err == nil {
		t.Fatal("zero history accepted")
	}
	if err := (Config{BackendURL: "http://x", History: 1, Backfill: -1}).validate(); err == nil {
		t.Fatal("negative backfill accepted")
	}
}
