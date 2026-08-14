package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-ebpf-filter/app/platform"

	"github.com/pelletier/go-toml/v2"
)

// ---- moved from backend/zz_merged_backend.go section detection_hooks.go ----

// hasNativeHookMarker checks whether the agent-ebpf hook marker is present in a config file.
func hasNativeHookMarker(cfgPath string) bool {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), hookMarker)
}

// isCodexHooksFeatureEnabled checks whether the codex_hooks feature flag is enabled.
func isCodexHooksFeatureEnabled(cfgPath string) bool {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return false
	}

	var cfg map[string]interface{}
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return false
	}

	features, _ := cfg["features"].(map[string]interface{})
	if features == nil {
		return false
	}

	enabled, _ := features["codex_hooks"].(bool)
	return enabled
}

// isNativeHookInstalled checks whether the agent-ebpf hook is present in the config
// and whether any required feature flags are enabled.
func isNativeHookInstalled(h HookDef) bool {
	if h.ID == "kiro" {
		return hasNativeHookMarker(h.NativeConfigPath) && isKiroManagedAgentSelected()
	}
	if !hasNativeHookMarker(h.NativeConfigPath) {
		return false
	}
	if h.NativeFeatureConfigPath != "" && !isCodexHooksFeatureEnabled(h.NativeFeatureConfigPath) {
		return false
	}
	return true
}

func isWrapperHookInstalled(cmd string) bool {
	p := getShellConfigPath()
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), fmt.Sprintf("alias %s=", cmd))
}

func isHookInstalled(h HookDef) bool {
	if h.HookType == HookTypeNative {
		return isNativeHookInstalled(h)
	}
	return isWrapperHookInstalled(h.TargetCmd)
}

func ensureCodexHooksFeatureEnabled(cfgPath string) error {
	if err := platform.MkdirAllAsRealUser(filepath.Dir(cfgPath), 0o755); err != nil {
		return err
	}

	var cfg map[string]interface{}
	if b, err := os.ReadFile(cfgPath); err == nil {
		_ = toml.Unmarshal(b, &cfg)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	features, _ := cfg["features"].(map[string]interface{})
	if features == nil {
		features = make(map[string]interface{})
	}
	features["codex_hooks"] = true
	cfg["features"] = features

	out, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return platform.WriteFileAsRealUser(cfgPath, out, 0o644)
}

func hookRelayScriptDir(h HookDef) string {
	if h.ID == "pi" || h.ID == "omp" {
		return filepath.Dir(filepath.Dir(h.NativeConfigPath))
	}
	return filepath.Join(filepath.Dir(h.NativeConfigPath), "hooks")
}

func hookRelayScriptPath(h HookDef) string {
	return filepath.Join(hookRelayScriptDir(h), hookMarker+"-"+h.ID+".sh")
}

func hookCommand(h HookDef, hookEvent string) string {
	scriptPath := hookRelayScriptPath(h)
	if strings.TrimSpace(hookEvent) == "" {
		return shellQuote(scriptPath)
	}
	return shellQuote(scriptPath) + " " + shellQuote(hookEvent)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func readJSONObjectFile(path string) (map[string]interface{}, error) {
	var cfg map[string]interface{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return make(map[string]interface{}), nil
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}
	return cfg, nil
}

func writeJSONObjectFile(path string, cfg map[string]interface{}) error {
	if err := platform.MkdirAllAsRealUser(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return platform.WriteFileAsRealUser(path, b, 0o644)
}
