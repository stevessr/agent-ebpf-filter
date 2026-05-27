package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func ensureHookRelayScript(h HookDef) (string, error) {
	scriptDir := filepath.Join(filepath.Dir(h.NativeConfigPath), "hooks")
	if err := mkdirAllAsRealUser(scriptDir, 0755); err != nil {
		return "", err
	}

	scriptPath := hookRelayScriptPath(h)
	scriptContent := buildHookRelayScript(h)

	if err := writeFileAsRealUser(scriptPath, []byte(scriptContent), 0755); err != nil {
		return "", err
	}
	return scriptPath, nil
}

func buildHookRelayScript(h HookDef) string {
	if h.ID == "antigravity" {
		return buildAntigravityHookRelayScript(h)
	}
	return buildGenericHookRelayScript(h)
}

func buildGenericHookRelayScript(h HookDef) string {
	hookSecret := runtimeSettingsStore.HookSecret(h.ID)
	return fmt.Sprintf(`#!/usr/bin/env bash
hook_event="${1:-${AGENT_EBPF_HOOK_EVENT:-}}"
tmp_file="$(mktemp "${TMPDIR:-/tmp}/agent-ebpf-hook.XXXXXX")" || exit 0
trap 'rm -f "$tmp_file"' EXIT
cat >"$tmp_file"
event_header=()
if [ -n "$hook_event" ]; then
  event_header=(-H "X-Agent-Hook-Event: $hook_event")
fi
curl -fsS -X POST '%s' \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-CLI: %s' \
  -H 'X-Agent-Hook-Secret: %s' \
  "${event_header[@]}" \
  --data-binary "@$tmp_file" \
  >/dev/null 2>&1 || true
`, resolveHookCallbackURL(), h.ID, hookSecret)
}

// installNativeHook injects a hook into the agent CLI's settings (JSON or TOML)
// that POSTs every tool call to our backend for inspection.
func installNativeHook(h HookDef) error {
	if h.ID == "kiro" {
		return installKiroNativeHook(h)
	}
	if h.ID == "antigravity" {
		return installAntigravityNativeHook(h)
	}

	cleanupLegacyCodexHookConfig(h)

	if h.NativeFeatureConfigPath != "" {
		if err := ensureCodexHooksFeatureEnabled(h.NativeFeatureConfigPath); err != nil {
			return err
		}
	}

	cfgPath := h.NativeConfigPath
	if err := mkdirAllAsRealUser(filepath.Dir(cfgPath), 0755); err != nil {
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
	if _, err := ensureHookRelayScript(h); err != nil {
		return err
	}
	hookCmd := hookCommand(h, h.NativeHookEvent)

	hookEntry := map[string]interface{}{
		"type":          "command",
		"command":       hookCmd,
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

	var (
		b   []byte
		err error
	)
	if h.ConfigFormat == ConfigFormatTOML {
		b, err = toml.Marshal(cfg)
	} else {
		b, err = json.MarshalIndent(cfg, "", "  ")
	}

	if err != nil {
		return err
	}
	return writeFileAsRealUser(cfgPath, b, 0644)
}

// uninstallNativeHook removes the agent-ebpf hook from settings.
func uninstallNativeHook(h HookDef) error {
	if h.ID == "kiro" {
		return uninstallKiroNativeHook(h)
	}
	if h.ID == "antigravity" {
		return uninstallAntigravityNativeHook(h)
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
	if err := writeFileAsRealUser(h.NativeConfigPath, out, 0644); err != nil {
		return err
	}
	_ = os.Remove(hookRelayScriptPath(h))

	cleanupLegacyCodexHookConfig(h)
	return nil
}
