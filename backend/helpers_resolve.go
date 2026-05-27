package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func resolveBackendPort() int {
	if raw := strings.TrimSpace(os.Getenv("AGENT_BACKEND_PORT")); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 {
			return port
		}
	}

	candidates := []string{".port"}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(sourceFile), ".port"))
	}

	for _, candidate := range candidates {
		b, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if port, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && port > 0 {
			return port
		}
	}

	return 8080
}

func resolveHookCallbackURL() string {
	if raw := strings.TrimSpace(os.Getenv("AGENT_HOOK_ENDPOINT")); raw != "" {
		return raw
	}
	return fmt.Sprintf("http://127.0.0.1:%d/hooks/event", resolveBackendPort())
}

func resolveWrapperPath() string {
	if override := os.Getenv("AGENT_WRAPPER_PATH"); override != "" {
		if info, err := os.Stat(override); err == nil && !info.IsDir() {
			return override
		}
	}

	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		sourceDir := filepath.Dir(sourceFile)
		candidates := []string{
			filepath.Join(sourceDir, "..", "agent-wrapper"),
			filepath.Join(sourceDir, "agent-wrapper"),
			filepath.Join(sourceDir, "..", "..", "agent-wrapper"),
		}
		for _, cnd := range candidates {
			if info, err := os.Stat(cnd); err == nil && !info.IsDir() {
				return cnd
			}
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		for _, rel := range []string{
			"agent-wrapper",
			"../agent-wrapper",
			"../../agent-wrapper",
			"../../../agent-wrapper",
		} {
			cnd := filepath.Clean(filepath.Join(cwd, rel))
			if info, err := os.Stat(cnd); err == nil && !info.IsDir() {
				return cnd
			}
		}
	}

	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		for _, rel := range []string{
			"agent-wrapper",
			"../agent-wrapper",
			"../../agent-wrapper",
		} {
			cnd := filepath.Clean(filepath.Join(execDir, rel))
			if info, err := os.Stat(cnd); err == nil && !info.IsDir() {
				return cnd
			}
		}
	}

	return ""
}

func resolveShellCandidate(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}

	if strings.ContainsRune(candidate, os.PathSeparator) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate
		}
		return ""
	}

	if resolved, err := exec.LookPath(candidate); err == nil {
		return resolved
	}

	for _, prefix := range []string{"/bin/", "/usr/bin/", "/usr/local/bin/"} {
		path := prefix + candidate
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return path
		}
	}

	return ""
}

func resolveShellPath(requested string) string {
	requested = strings.TrimSpace(requested)

	switch strings.ToLower(requested) {
	case "", "auto":
		for _, candidate := range []string{"fish", "zsh", "bash", "ash", "sh"} {
			if resolved := resolveShellCandidate(candidate); resolved != "" {
				return resolved
			}
		}
		return ""
	case "system", "env":
		if resolved := resolveShellCandidate(os.Getenv("SHELL")); resolved != "" {
			return resolved
		}
		for _, candidate := range []string{"fish", "zsh", "bash", "ash", "sh"} {
			if resolved := resolveShellCandidate(candidate); resolved != "" {
				return resolved
			}
		}
		return ""
	default:
		return resolveShellCandidate(requested)
	}
}

func resolveShellWorkDir() string {
	if override := os.Getenv("AGENT_SHELL_DIR"); override != "" {
		if info, err := os.Stat(override); err == nil && info.IsDir() {
			return override
		}
	}

	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Dir(filepath.Dir(sourceFile))
		if info, err := os.Stat(repoRoot); err == nil && info.IsDir() {
			return repoRoot
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}

	if home, err := os.UserHomeDir(); err == nil {
		return home
	}

	return "/"
}

func setEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	replaced := false
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			replaced = true
			break
		}
	}
	if !replaced {
		env = append(env, prefix+value)
	}
	return env
}

func writePortFile(actualPort int) {
	data := []byte(fmt.Sprintf("%d", actualPort))
	_ = os.WriteFile(".port", data, 0644)

	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		backendDir := filepath.Dir(sourceFile)
		_ = os.WriteFile(filepath.Join(backendDir, ".port"), data, 0644)
	}
}
