package platform

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

func OriginalInvokerIDs() (uid, gid uint32, ok bool) {
	if uidStr := os.Getenv("SUDO_UID"); uidStr != "" {
		gidStr := os.Getenv("SUDO_GID")
		if gidStr == "" { return 0, 0, false }
		pUid, e1 := strconv.ParseUint(uidStr, 10, 32)
		pGid, e2 := strconv.ParseUint(gidStr, 10, 32)
		if e1 != nil || e2 != nil { return 0, 0, false }
		return uint32(pUid), uint32(pGid), true
	}
	if uidStr := os.Getenv("PKEXEC_UID"); uidStr != "" {
		u, err := user.LookupId(uidStr)
		if err != nil { return 0, 0, false }
		pUid, e1 := strconv.ParseUint(uidStr, 10, 32)
		pGid, e2 := strconv.ParseUint(u.Gid, 10, 32)
		if e1 != nil || e2 != nil { return 0, 0, false }
		return uint32(pUid), uint32(pGid), true
	}
	return 0, 0, false
}

func WriteFileAsRealUser(path string, data []byte, perm os.FileMode) error {
	if err := os.WriteFile(path, data, perm); err != nil { return err }
	if os.Getuid() == 0 {
		if uid, gid, ok := OriginalInvokerIDs(); ok { _ = os.Chown(path, int(uid), int(gid)) }
	}
	return nil
}

func MkdirAllAsRealUser(path string, perm os.FileMode) error {
	if err := os.MkdirAll(path, perm); err != nil { return err }
	if os.Getuid() == 0 {
		if uid, gid, ok := OriginalInvokerIDs(); ok { _ = os.Chown(path, int(uid), int(gid)) }
	}
	return nil
}

var (
	getRealHomeOnce sync.Once
	getRealHomeVal  string
)

func GetRealHomeDir() string {
	getRealHomeOnce.Do(func() {
		if h := os.Getenv("AGENT_REAL_HOME"); h != "" { getRealHomeVal = h; return }
		if os.Getuid() == 0 {
			for _, env := range []string{"SUDO_USER", "PKEXEC_UID"} {
				if v := os.Getenv(env); v != "" {
					if u, err := user.Lookup(v); err == nil { getRealHomeVal = u.HomeDir; return }
				}
			}
			if home := os.Getenv("HOME"); home != "" && home != "/root" { getRealHomeVal = home; return }
		}
		h, _ := os.UserHomeDir()
		if h == "" || h == "/root" {
			if entries, err := os.ReadDir("/home"); err == nil {
				for _, e := range entries {
					if e.IsDir() && e.Name() != "lost+found" { getRealHomeVal = filepath.Join("/home", e.Name()); return }
				}
			}
		}
		getRealHomeVal = h
	})
	return getRealHomeVal
}

func RuntimeSettingsDir() string { return filepath.Join(GetRealHomeDir(), ".config", "agent-ebpf-filter") }
func RuntimeSettingsPath() string { return filepath.Join(RuntimeSettingsDir(), "runtime.json") }
func DefaultEventLogPath() string { return filepath.Join(RuntimeSettingsDir(), "events.jsonl") }

func FirstNonEmpty(values ...string) string {
	for _, v := range values { if strings.TrimSpace(v) != "" { return strings.TrimSpace(v) } }
	return ""
}

func ParseStringField(extraInfo, key string) string {
	needle := key + "="
	for _, part := range strings.Fields(strings.ReplaceAll(extraInfo, ",", " ")) {
		if strings.HasPrefix(part, needle) { return strings.TrimSpace(strings.TrimPrefix(part, needle)) }
	}
	return ""
}

func ParseUintField(extraInfo, key string) uint32 {
	val := ParseStringField(extraInfo, key)
	if val == "" { return 0 }
	var r uint64
	fmt.Sscanf(val, "%d", &r)
	return uint32(r)
}

func ParseFloatField(extraInfo, key string) float64 {
	val := ParseStringField(extraInfo, key)
	if val == "" { return 0 }
	var r float64
	fmt.Sscanf(val, "%f", &r)
	return r
}

func PluginsRootDir() string { return filepath.Join(RuntimeSettingsDir(), "plugins") }
func ResolveBackendPort() int {
	if raw := strings.TrimSpace(os.Getenv("AGENT_BACKEND_PORT")); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 { return port }
	}
	candidates := []string{".port"}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(sourceFile), ".port"))
	}
	for _, candidate := range candidates {
		b, err := os.ReadFile(candidate)
		if err != nil { continue }
		if port, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && port > 0 { return port }
	}
	return 8080
}

func ResolveHookCallbackURL() string {
	if raw := strings.TrimSpace(os.Getenv("AGENT_HOOK_ENDPOINT")); raw != "" { return raw }
	return fmt.Sprintf("http://127.0.0.1:%d/hooks/event", ResolveBackendPort())
}

func ResolveWrapperPath() string {
	if override := os.Getenv("AGENT_WRAPPER_PATH"); override != "" {
		if info, err := os.Stat(override); err == nil && !info.IsDir() { return override }
	}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		sourceDir := filepath.Dir(sourceFile)
		for _, rel := range []string{
			filepath.Join(sourceDir, "..", "agent-wrapper"),
			filepath.Join(sourceDir, "agent-wrapper"),
			filepath.Join(sourceDir, "..", "..", "agent-wrapper"),
		} {
			if info, err := os.Stat(rel); err == nil && !info.IsDir() { return rel }
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		for _, rel := range []string{"agent-wrapper", "../agent-wrapper", "../../agent-wrapper", "../../../agent-wrapper"} {
			cnd := filepath.Clean(filepath.Join(cwd, rel))
			if info, err := os.Stat(cnd); err == nil && !info.IsDir() { return cnd }
		}
	}
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		for _, rel := range []string{"agent-wrapper", "../agent-wrapper", "../../agent-wrapper"} {
			cnd := filepath.Clean(filepath.Join(execDir, rel))
			if info, err := os.Stat(cnd); err == nil && !info.IsDir() { return cnd }
		}
	}
	return ""
}

func ResolveShellPath(requested string) string {
	requested = strings.TrimSpace(requested)
	switch strings.ToLower(requested) {
	case "", "auto":
		for _, candidate := range []string{"fish", "zsh", "bash", "ash", "sh"} {
			if r := resolveShellCandidate(candidate); r != "" { return r }
		}
		return ""
	case "system", "env":
		if r := resolveShellCandidate(os.Getenv("SHELL")); r != "" { return r }
		for _, candidate := range []string{"fish", "zsh", "bash", "ash", "sh"} {
			if r := resolveShellCandidate(candidate); r != "" { return r }
		}
		return ""
	}
	return resolveShellCandidate(requested)
}

func resolveShellCandidate(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" { return "" }
	if strings.ContainsRune(candidate, os.PathSeparator) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 { return candidate }
		return ""
	}
	if resolved, err := exec.LookPath(candidate); err == nil { return resolved }
	for _, prefix := range []string{"/bin/", "/usr/bin/", "/usr/local/bin/"} {
		path := prefix + candidate
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0111 != 0 { return path }
	}
	return ""
}

func ResolveShellWorkDir() string {
	if override := os.Getenv("AGENT_SHELL_DIR"); override != "" {
		if info, err := os.Stat(override); err == nil && info.IsDir() { return override }
	}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Dir(filepath.Dir(sourceFile))
		if info, err := os.Stat(repoRoot); err == nil && info.IsDir() { return repoRoot }
	}
	if cwd, err := os.Getwd(); err == nil { return cwd }
	if home, err := os.UserHomeDir(); err == nil { return home }
	return "/"
}

func SetEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	replaced := false
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			replaced = true
			break
		}
	}
	if !replaced { env = append(env, prefix+value) }
	return env
}

func WritePortFile(actualPort int) {
	data := []byte(fmt.Sprintf("%d", actualPort))
	_ = os.WriteFile(".port", data, 0644)
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		backendDir := filepath.Dir(sourceFile)
		_ = os.WriteFile(filepath.Join(backendDir, ".port"), data, 0644)
	}
}
