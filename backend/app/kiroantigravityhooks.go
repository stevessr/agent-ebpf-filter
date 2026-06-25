package app

import (
	"agent-ebpf-filter/app/platform"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ---- moved from backend/zz_merged_backend.go section kiroantigravityhooks.go ----

// ── Kiro-specific hook functions ──────────────────────────────────────

func kiroSettingsPath() string {
	return filepath.Join(platform.GetRealHomeDir(), ".kiro", "settings", "cli.json")
}

func kiroHookStatePath() string {
	return filepath.Join(platform.GetRealHomeDir(), ".kiro", "settings", "agent-ebpf-hook-state.json")
}

func kiroManagedAgentPath() string {
	return filepath.Join(platform.GetRealHomeDir(), ".kiro", "agents", kiroManagedAgent+".json")
}

func ensureKiroManagedAgentExists() error {
	agentPath := kiroManagedAgentPath()
	if _, err := os.Stat(agentPath); err == nil {
		return nil
	}

	agentsDir := filepath.Dir(agentPath)
	if err := platform.MkdirAllAsRealUser(agentsDir, 0755); err != nil {
		return err
	}

	cmd := exec.Command("kiro-cli", "agent", "create", kiroManagedAgent, "--from", "kiro_default", "-d", agentsDir)
	configureCommandForRealUser(cmd)
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = setEnvValue(cmd.Env, "HOME", platform.GetRealHomeDir())
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
	if err := platform.MkdirAllAsRealUser(filepath.Dir(kiroHookStatePath()), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return platform.WriteFileAsRealUser(kiroHookStatePath(), b, 0644)
}

func isKiroManagedAgentSelected() bool {
	settings, err := readJSONObjectFile(kiroSettingsPath())
	if err != nil {
		return false
	}
	agentName, _ := settings["chat.defaultAgent"].(string)
	return agentName == kiroManagedAgent
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

	if _, err := ensureHookRelayScript(h); err != nil {
		return err
	}
	hookCmd := hookCommand(h, h.NativeHookEvent)

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
		"command": hookCmd,
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

// ── Antigravity-specific hook functions ───────────────────────────────

func antigravityPluginManifestPath(h HookDef) string {
	return filepath.Join(filepath.Dir(h.NativeConfigPath), "plugin.json")
}

func ensureAntigravityPluginManifest(h HookDef) error {
	manifestPath := antigravityPluginManifestPath(h)
	if err := platform.MkdirAllAsRealUser(filepath.Dir(manifestPath), 0755); err != nil {
		return err
	}
	if _, err := os.Stat(manifestPath); err == nil {
		return nil
	}
	manifest := map[string]interface{}{
		"name":        hookMarker,
		"displayName": "agent-ebpf hook relay",
		"version":     "1.0.0",
		"description": "Forwards Antigravity CLI hook telemetry to agent-ebpf-filter.",
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return platform.WriteFileAsRealUser(manifestPath, data, 0644)
}

func isAntigravityMatcherEvent(event string) bool {
	switch event {
	case "PreToolUse", "PostToolUse":
		return true
	default:
		return false
	}
}

func filterAntigravityHookEntries(entries []interface{}) []interface{} {
	filtered := make([]interface{}, 0, len(entries))
	for _, entry := range entries {
		em, ok := entry.(map[string]interface{})
		if !ok {
			filtered = append(filtered, entry)
			continue
		}
		if cmd, _ := em["command"].(string); strings.Contains(cmd, hookMarker) {
			continue
		}
		if hs, _ := em["hooks"].([]interface{}); len(hs) > 0 {
			isOurs := false
			for _, hookEntry := range hs {
				hm, ok := hookEntry.(map[string]interface{})
				if !ok {
					continue
				}
				if cmd, _ := hm["command"].(string); strings.Contains(cmd, hookMarker) {
					isOurs = true
					break
				}
			}
			if isOurs {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func installAntigravityNativeHook(h HookDef) error {
	if err := ensureAntigravityPluginManifest(h); err != nil {
		return err
	}
	cfg, err := readJSONObjectFile(h.NativeConfigPath)
	if err != nil {
		return err
	}
	if _, err := ensureHookRelayScript(h); err != nil {
		return err
	}

	hookDef, _ := cfg[hookMarker].(map[string]interface{})
	if hookDef == nil {
		hookDef = make(map[string]interface{})
	}
	hookDef["enabled"] = true

	eventName := h.NativeHookEvent
	eventEntries, _ := hookDef[eventName].([]interface{})
	filtered := filterAntigravityHookEntries(eventEntries)
	commandEntry := map[string]interface{}{
		"type":    "command",
		"command": hookCommand(h, eventName),
		"timeout": 5,
	}
	if isAntigravityMatcherEvent(eventName) {
		matcher := strings.TrimSpace(h.NativeMatcher)
		if matcher == "" {
			matcher = "*"
		}
		hookDef[eventName] = append(filtered, map[string]interface{}{
			"matcher": matcher,
			"hooks":   []interface{}{commandEntry},
		})
	} else {
		hookDef[eventName] = append(filtered, commandEntry)
	}

	cfg[hookMarker] = hookDef
	return writeJSONObjectFile(h.NativeConfigPath, cfg)
}

func uninstallAntigravityNativeHook(h HookDef) error {
	cfg, err := readJSONObjectFile(h.NativeConfigPath)
	if err != nil {
		_ = os.Remove(hookRelayScriptPath(h))
		return nil
	}
	hookDef, _ := cfg[hookMarker].(map[string]interface{})
	if hookDef != nil {
		for _, eventName := range []string{"PreToolUse", "PostToolUse", "PreInvocation", "PostInvocation", "Stop"} {
			eventEntries, _ := hookDef[eventName].([]interface{})
			if len(eventEntries) == 0 {
				continue
			}
			filtered := filterAntigravityHookEntries(eventEntries)
			if len(filtered) == 0 {
				delete(hookDef, eventName)
			} else {
				hookDef[eventName] = filtered
			}
		}
		if len(hookDef) == 0 || (len(hookDef) == 1 && hookDef["enabled"] != nil) {
			delete(cfg, hookMarker)
		} else {
			cfg[hookMarker] = hookDef
		}
	}
	if err := writeJSONObjectFile(h.NativeConfigPath, cfg); err != nil {
		return err
	}
	_ = os.Remove(hookRelayScriptPath(h))
	return nil
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

func buildAntigravityHookRelayScript(h HookDef) string {
	hookSecret := runtimeSettingsStore.HookSecret(h.ID)
	return fmt.Sprintf(`#!/usr/bin/env bash
hook_event="${1:-${AGENT_EBPF_HOOK_EVENT:-}}"
tmp_file="$(mktemp "${TMPDIR:-/tmp}/agent-ebpf-hook.XXXXXX")" || {
  case "$hook_event" in
    PreToolUse) printf '{"decision":"allow","reason":"agent-ebpf relay unavailable"}\n' ;;
    Stop) printf '{"decision":"allow","reason":""}\n' ;;
    PreInvocation) printf '{"injectSteps":[]}\n' ;;
    PostInvocation) printf '{"injectSteps":[],"terminationBehavior":""}\n' ;;
    *) printf '{}\n' ;;
  esac
  exit 0
}
trap 'rm -f "$tmp_file"' EXIT
cat >"$tmp_file"
curl -fsS -X POST '%s' \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-CLI: antigravity' \
  -H "X-Agent-Hook-Event: $hook_event" \
  -H 'X-Agent-Hook-Secret: %s' \
  --data-binary "@$tmp_file" \
  >/dev/null 2>&1 || true

case "$hook_event" in
  PreToolUse)
    printf '{"decision":"allow","reason":"agent-ebpf telemetry recorded"}\n'
    ;;
  Stop)
    printf '{"decision":"allow","reason":""}\n'
    ;;
  PreInvocation)
    printf '{"injectSteps":[]}\n'
    ;;
  PostInvocation)
    printf '{"injectSteps":[],"terminationBehavior":""}\n'
    ;;
  *)
    printf '{}\n'
    ;;
esac
`, resolveHookCallbackURL(), hookSecret)
}
