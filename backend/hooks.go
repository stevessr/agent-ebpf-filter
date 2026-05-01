package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
	"github.com/pelletier/go-toml/v2"
)

func handleNativeHookEvent(c *gin.Context) {
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	toolName, _ := payload["tool_name"].(string)
	hookEvent, _ := payload["hook_event_name"].(string)
	toolInput, _ := payload["tool_input"].(map[string]interface{})

	if toolName == "" {
		toolName, _ = payload["tool"].(string)
	}
	if hookEvent == "" {
		hookEvent, _ = payload["event"].(string)
	}

	path := ""
	if toolInput != nil {
		if cmd, ok := toolInput["command"].(string); ok {
			path = cmd
		} else if fp, ok := toolInput["file_path"].(string); ok {
			path = fp
		} else if args, ok := toolInput["arguments"].([]interface{}); ok && len(args) > 0 {
			path = fmt.Sprintf("%v", args)
		}
	}

	tag := "Native Hook"
	sourceCLI := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Agent-CLI")))
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	if sourceCLI == "claude" || strings.Contains(ua, "claude") {
		tag = "Claude Code"
	} else if sourceCLI == "gemini" || strings.Contains(ua, "gemini") {
		tag = "Gemini CLI"
	} else if sourceCLI == "codex" || strings.Contains(ua, "codex") {
		tag = "Codex"
	} else if sourceCLI == "copilot" || strings.Contains(ua, "copilot") || strings.Contains(ua, "gh-copilot") {
		tag = "GitHub Copilot"
	} else if sourceCLI == "kiro" || strings.Contains(ua, "kiro") {
		tag = "Kiro CLI"
	} else if sourceCLI == "augment" || strings.Contains(ua, "augment") || strings.Contains(ua, "auggie") {
		tag = "Augment"
	} else {
		if hookEvent == "BeforeTool" {
			tag = "Gemini CLI"
		} else if hookEvent == "preToolUse" {
			tag = "GitHub Copilot"
		} else if hookEvent == "agentSpawn" || hookEvent == "userPromptSubmit" || hookEvent == "stop" {
			tag = "Kiro CLI"
		}
	}

	broadcast <- &pb.Event{
		Type:      "native_hook",
		EventType: pb.EventType_NATIVE_HOOK,
		Tag:       tag,
		Comm:      fmt.Sprintf("%s:%s", hookEvent, toolName),
		Path:      path,
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func hasNativeHookMarker(cfgPath string) bool {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), hookMarker)
}

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
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
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
	return os.WriteFile(cfgPath, out, 0644)
}

func hookRelayScriptPath(h HookDef) string {
	return filepath.Join(filepath.Dir(h.NativeConfigPath), "hooks", hookMarker+"-"+h.ID+".sh")
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
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func kiroSettingsPath() string {
	return filepath.Join(getRealHomeDir(), ".kiro", "settings", "cli.json")
}

func kiroHookStatePath() string {
	return filepath.Join(getRealHomeDir(), ".kiro", "settings", "agent-ebpf-hook-state.json")
}

func kiroManagedAgentPath() string {
	return filepath.Join(getRealHomeDir(), ".kiro", "agents", kiroManagedAgent+".json")
}

func ensureKiroManagedAgentExists() error {
	agentPath := kiroManagedAgentPath()
	if _, err := os.Stat(agentPath); err == nil {
		return nil
	}

	agentsDir := filepath.Dir(agentPath)
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return err
	}

	cmd := exec.Command("kiro-cli", "agent", "create", kiroManagedAgent, "--from", "kiro_default", "-d", agentsDir)
	configureCommandForRealUser(cmd)
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = setEnvValue(cmd.Env, "HOME", getRealHomeDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create managed Kiro agent from kiro_default: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func readKiroHookState() (kiroHookState, error) {
	state := kiroHookState{}
	b, err := os.ReadFile(kiroHookStatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if err := json.Unmarshal(b, &state); err != nil {
		return state, err
	}
	return state, nil
}

func writeKiroHookState(state kiroHookState) error {
	if err := os.MkdirAll(filepath.Dir(kiroHookStatePath()), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(kiroHookStatePath(), b, 0644)
}

func isKiroManagedAgentSelected() bool {
	settings, err := readJSONObjectFile(kiroSettingsPath())
	if err != nil {
		return false
	}
	agentName, _ := settings["chat.defaultAgent"].(string)
	return agentName == kiroManagedAgent
}

func ensureHookRelayScript(h HookDef) (string, error) {
	scriptDir := filepath.Join(filepath.Dir(h.NativeConfigPath), "hooks")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		return "", err
	}

	scriptPath := hookRelayScriptPath(h)
	scriptContent := fmt.Sprintf(`#!/usr/bin/env bash
tmp_file="$(mktemp "${TMPDIR:-/tmp}/agent-ebpf-hook.XXXXXX")" || exit 0
trap 'rm -f "$tmp_file"' EXIT
cat >"$tmp_file"
curl -fsS -X POST '%s' \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-CLI: %s' \
  --data-binary "@$tmp_file" \
  >/dev/null 2>&1 || true
`, resolveHookCallbackURL(), h.ID)

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return "", err
	}
	return scriptPath, nil
}

func installKiroDefaultAgentSelection() error {
	settings, err := readJSONObjectFile(kiroSettingsPath())
	if err != nil {
		return err
	}

	currentDefault, _ := settings["chat.defaultAgent"].(string)
	if currentDefault != kiroManagedAgent {
		if err := writeKiroHookState(kiroHookState{PreviousDefaultAgent: currentDefault}); err != nil {
			return err
		}
	}

	settings["chat.defaultAgent"] = kiroManagedAgent
	return writeJSONObjectFile(kiroSettingsPath(), settings)
}

func restoreKiroDefaultAgentSelection() error {
	settings, err := readJSONObjectFile(kiroSettingsPath())
	if err != nil {
		return err
	}

	state, err := readKiroHookState()
	if err != nil {
		return err
	}

	currentDefault, _ := settings["chat.defaultAgent"].(string)
	if currentDefault == kiroManagedAgent {
		if state.PreviousDefaultAgent != "" {
			settings["chat.defaultAgent"] = state.PreviousDefaultAgent
		} else {
			delete(settings, "chat.defaultAgent")
		}
		if err := writeJSONObjectFile(kiroSettingsPath(), settings); err != nil {
			return err
		}
	}

	_ = os.Remove(kiroHookStatePath())
	return nil
}

func installKiroNativeHook(h HookDef) error {
	if err := ensureKiroManagedAgentExists(); err != nil {
		return err
	}

	cfg, err := readJSONObjectFile(h.NativeConfigPath)
	if err != nil {
		return err
	}

	scriptPath, err := ensureHookRelayScript(h)
	if err != nil {
		return err
	}
	hookCommand := fmt.Sprintf(`'%s'`, scriptPath)

	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = make(map[string]interface{})
	}
	eventHooks, _ := hooks[h.NativeHookEvent].([]interface{})

	filtered := make([]interface{}, 0, len(eventHooks))
	for _, entry := range eventHooks {
		em, ok := entry.(map[string]interface{})
		if !ok {
			filtered = append(filtered, entry)
			continue
		}
		cmd, _ := em["command"].(string)
		if strings.Contains(cmd, hookMarker) {
			continue
		}
		filtered = append(filtered, entry)
	}

	hookEntry := map[string]interface{}{
		"command": hookCommand,
	}
	if h.NativeMatcher != "" {
		hookEntry["matcher"] = h.NativeMatcher
	}

	hooks[h.NativeHookEvent] = append(filtered, hookEntry)
	cfg["hooks"] = hooks
	if err := writeJSONObjectFile(h.NativeConfigPath, cfg); err != nil {
		return err
	}
	return installKiroDefaultAgentSelection()
}

func uninstallKiroNativeHook(h HookDef) error {
	cfg, err := readJSONObjectFile(h.NativeConfigPath)
	if err != nil {
		return err
	}

	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks != nil {
		eventHooks, _ := hooks[h.NativeHookEvent].([]interface{})
		filtered := make([]interface{}, 0, len(eventHooks))
		for _, entry := range eventHooks {
			em, ok := entry.(map[string]interface{})
			if !ok {
				filtered = append(filtered, entry)
				continue
			}
			cmd, _ := em["command"].(string)
			if strings.Contains(cmd, hookMarker) {
				continue
			}
			filtered = append(filtered, entry)
		}

		if len(filtered) == 0 {
			delete(hooks, h.NativeHookEvent)
		} else {
			hooks[h.NativeHookEvent] = filtered
		}
		if len(hooks) == 0 {
			delete(cfg, "hooks")
		} else {
			cfg["hooks"] = hooks
		}
		if err := writeJSONObjectFile(h.NativeConfigPath, cfg); err != nil {
			return err
		}
	}

	_ = os.Remove(hookRelayScriptPath(h))
	return restoreKiroDefaultAgentSelection()
}

func cleanupLegacyCodexHookConfig(h HookDef) {
	if h.ID != "codex" || h.NativeFeatureConfigPath == "" {
		return
	}

	legacyHookConfig := HookDef{
		NativeConfigPath: h.NativeFeatureConfigPath,
		NativeHookEvent:  h.NativeHookEvent,
		ConfigFormat:     ConfigFormatTOML,
	}
	if err := uninstallNativeHook(legacyHookConfig); err != nil {
		log.Printf("[WARN] failed to clean up legacy Codex hook config from %s: %v", h.NativeFeatureConfigPath, err)
	}
}

// installNativeHook injects a hook into the agent CLI's settings (JSON or TOML)
// that POSTs every tool call to our backend for inspection.
func installNativeHook(h HookDef) error {
	if h.ID == "kiro" {
		return installKiroNativeHook(h)
	}

	cleanupLegacyCodexHookConfig(h)

	if h.NativeFeatureConfigPath != "" {
		if err := ensureCodexHooksFeatureEnabled(h.NativeFeatureConfigPath); err != nil {
			return err
		}
	}

	cfgPath := h.NativeConfigPath
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return err
	}

	// Read existing config (may not exist yet).
	var cfg map[string]interface{}
	if b, err := os.ReadFile(cfgPath); err == nil {
		if h.ConfigFormat == ConfigFormatTOML {
			_ = toml.Unmarshal(b, &cfg)
		} else {
			_ = json.Unmarshal(b, &cfg)
		}
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	// Build the hook entry.
	scriptPath, err := ensureHookRelayScript(h)
	if err != nil {
		return err
	}
	hookCommand := fmt.Sprintf(`'%s'`, scriptPath)

	hookEntry := map[string]interface{}{
		"type":          "command",
		"command":       hookCommand,
		"statusMessage": "agent-ebpf-hook-active: inspecting...",
	}
	switch h.ID {
	case "codex":
		// Codex doesn't support async hooks
	case "augment":
		// Augment uses `timeout` (ms) rather than async
		hookEntry["timeout"] = 5000
	default:
		hookEntry["async"] = true
	}
	matcher := map[string]interface{}{"hooks": []interface{}{hookEntry}}
	if h.NativeMatcher != "" {
		matcher["matcher"] = h.NativeMatcher
	} else {
		matcher["matcher"] = ""
	}

	// Merge into existing hooks[eventName] array.
	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = make(map[string]interface{})
	}
	eventHooks, _ := hooks[h.NativeHookEvent].([]interface{})

	// Remove any existing agent-ebpf-hook entry to avoid duplicates.
	filtered := []interface{}{}
	for _, m := range eventHooks {
		if mm, ok := m.(map[string]interface{}); ok {
			hs, _ := mm["hooks"].([]interface{})
			isOurs := false
			for _, h := range hs {
				if hm, ok := h.(map[string]interface{}); ok {
					if cmd, _ := hm["command"].(string); strings.Contains(cmd, hookMarker) {
						isOurs = true
					}
				}
			}
			if !isOurs {
				filtered = append(filtered, m)
			}
		}
	}
	hooks[h.NativeHookEvent] = append(filtered, matcher)
	cfg["hooks"] = hooks

	var b []byte
	if h.ConfigFormat == ConfigFormatTOML {
		b, err = toml.Marshal(cfg)
	} else {
		b, err = json.MarshalIndent(cfg, "", "  ")
	}

	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, b, 0644)
}

// uninstallNativeHook removes the agent-ebpf hook from settings.
func uninstallNativeHook(h HookDef) error {
	if h.ID == "kiro" {
		return uninstallKiroNativeHook(h)
	}

	b, err := os.ReadFile(h.NativeConfigPath)
	if err != nil {
		_ = os.Remove(hookRelayScriptPath(h))
		cleanupLegacyCodexHookConfig(h)
		return nil // nothing to do
	}
	var cfg map[string]interface{}
	if h.ConfigFormat == ConfigFormatTOML {
		if err := toml.Unmarshal(b, &cfg); err != nil {
			return err
		}
	} else {
		if err := json.Unmarshal(b, &cfg); err != nil {
			return err
		}
	}

	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks == nil {
		_ = os.Remove(hookRelayScriptPath(h))
		cleanupLegacyCodexHookConfig(h)
		return nil
	}
	eventHooks, _ := hooks[h.NativeHookEvent].([]interface{})
	filtered := []interface{}{}
	for _, m := range eventHooks {
		if mm, ok := m.(map[string]interface{}); ok {
			hs, _ := mm["hooks"].([]interface{})
			isOurs := false
			for _, h := range hs {
				if hm, ok := h.(map[string]interface{}); ok {
					if cmd, _ := hm["command"].(string); strings.Contains(cmd, hookMarker) {
						isOurs = true
					}
				}
			}
			if !isOurs {
				filtered = append(filtered, m)
			}
		}
	}
	if len(filtered) == 0 {
		delete(hooks, h.NativeHookEvent)
	} else {
		hooks[h.NativeHookEvent] = filtered
	}
	if len(hooks) == 0 {
		delete(cfg, "hooks")
	}

	var out []byte
	if h.ConfigFormat == ConfigFormatTOML {
		out, err = toml.Marshal(cfg)
	} else {
		out, err = json.MarshalIndent(cfg, "", "  ")
	}

	if err != nil {
		return err
	}
	if err := os.WriteFile(h.NativeConfigPath, out, 0644); err != nil {
		return err
	}
	_ = os.Remove(hookRelayScriptPath(h))

	cleanupLegacyCodexHookConfig(h)
	return nil
}
