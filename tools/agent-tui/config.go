package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is everything the TUI needs to reach one backend.
type Config struct {
	// BackendURL is the HTTP origin of the backend, e.g. http://127.0.0.1:8080.
	BackendURL string
	// Token is sent as X-API-KEY. Empty is fine for dev-mode backends.
	Token string
	// History is the number of events kept in memory.
	History int
	// Backfill is how many recent events to load before streaming.
	Backfill int
}

const (
	defaultBackendPort = 8080
	defaultHistory     = 5000
	defaultBackfill    = 200
)

// resolveBackendURL picks the backend origin from, in order: the explicit
// flag, AGENT_BACKEND_URL, AGENT_BACKEND_PORT, a backend/.port file written by
// a running dev backend (searched upward from cwd), and finally port 8080.
func resolveBackendURL(flagValue string, lookupEnv func(string) string, cwd string) (string, error) {
	if raw := strings.TrimSpace(flagValue); raw != "" {
		return normalizeBackendURL(raw)
	}
	if raw := strings.TrimSpace(lookupEnv("AGENT_BACKEND_URL")); raw != "" {
		return normalizeBackendURL(raw)
	}
	if raw := strings.TrimSpace(lookupEnv("AGENT_BACKEND_PORT")); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 {
			return fmt.Sprintf("http://127.0.0.1:%d", port), nil
		}
	}
	if port := portFromDotPortFile(cwd); port > 0 {
		return fmt.Sprintf("http://127.0.0.1:%d", port), nil
	}
	return fmt.Sprintf("http://127.0.0.1:%d", defaultBackendPort), nil
}

func normalizeBackendURL(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid backend url %q: %w", raw, err)
	}
	switch parsed.Scheme {
	case "http", "https":
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	default:
		return "", fmt.Errorf("unsupported backend url scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("backend url %q has no host", raw)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// portFromDotPortFile walks from dir upward looking for backend/.port or .port.
func portFromDotPortFile(dir string) int {
	dir = filepath.Clean(dir)
	for i := 0; i < 8 && dir != ""; i++ {
		for _, candidate := range []string{filepath.Join(dir, "backend", ".port"), filepath.Join(dir, ".port")} {
			raw, err := os.ReadFile(candidate)
			if err != nil {
				continue
			}
			if port, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && port > 0 {
				return port
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return 0
}

// resolveToken picks the API token from the flag, AGENT_ACCESS_TOKEN, or the
// backend's persisted runtime.json (~/.config/agent-ebpf-filter/runtime.json).
func resolveToken(flagValue string, lookupEnv func(string) string, homeDir string) string {
	if raw := strings.TrimSpace(flagValue); raw != "" {
		return raw
	}
	if raw := strings.TrimSpace(lookupEnv("AGENT_ACCESS_TOKEN")); raw != "" {
		return raw
	}
	if homeDir == "" {
		return ""
	}
	token, _ := tokenFromRuntimeSettings(filepath.Join(homeDir, ".config", "agent-ebpf-filter", "runtime.json"))
	return token
}

func tokenFromRuntimeSettings(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var settings struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return strings.TrimSpace(settings.AccessToken), nil
}

// realHomeDir prefers the invoking user's home when running under sudo so the
// token file written by the (root) backend for that user is still found.
func realHomeDir() string {
	for _, env := range []string{"SUDO_USER", "PKEXEC_UID"} {
		if v := os.Getenv(env); v != "" {
			if u, err := user.Lookup(v); err == nil && u.HomeDir != "" {
				return u.HomeDir
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

func (c Config) validate() error {
	if c.BackendURL == "" {
		return errors.New("backend url is required")
	}
	if c.History <= 0 {
		return errors.New("history must be positive")
	}
	if c.Backfill < 0 {
		return errors.New("backfill must not be negative")
	}
	return nil
}
