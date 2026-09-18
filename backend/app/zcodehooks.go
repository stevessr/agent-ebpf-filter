package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-ebpf-filter/app/platform"
)

var zcodeHookEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"PreToolUse",
	"PermissionRequest",
	"PostToolUse",
	"PostToolUseFailure",
	"Stop",
}

func zcodeEventMatcher(event string) string {
	switch event {
	case "SessionStart":
		return "startup|clear|compact"
	case "PreToolUse", "PermissionRequest", "PostToolUse", "PostToolUseFailure":
		return "*"
	default:
		return ""
	}
}

func filterZCodeHookEntries(entries []interface{}) []interface{} {
	filtered := make([]interface{}, 0, len(entries))
	for _, entry := range entries {
		em, ok := entry.(map[string]interface{})
		if !ok {
			filtered = append(filtered, entry)
			continue
		}
		hooks, _ := em["hooks"].([]interface{})
		ours := false
		for _, hook := range hooks {
			hm, ok := hook.(map[string]interface{})
			if !ok {
				continue
			}
			if cmd, _ := hm["command"].(string); strings.Contains(cmd, hookMarker) {
				ours = true
				break
			}
		}
		if !ours {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func zcodeHookStatePath(h HookDef) string {
	return filepath.Join(filepath.Dir(h.NativeConfigPath), hookMarker+"-zcode-state.json")
}

func captureZCodeHooksEnabledState(h HookDef, hooks map[string]interface{}) error {
	statePath := zcodeHookStatePath(h)
	if _, err := os.Stat(statePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	state := map[string]interface{}{"had_enabled": false}
	if previous, ok := hooks["enabled"]; ok {
		state["had_enabled"] = true
		state["enabled"] = previous
	}
	return writeJSONObjectFile(statePath, state)
}

func restoreZCodeHooksEnabledState(h HookDef, hooks map[string]interface{}) error {
	statePath := zcodeHookStatePath(h)
	state, err := readJSONObjectFile(statePath)
	if err != nil {
		return err
	}
	if len(state) == 0 {
		return nil
	}

	// Only restore when the current value is still the value installed by us.
	// If the user changed hooks.enabled while the integration was installed,
	// preserve that explicit choice.
	if current, ok := hooks["enabled"].(bool); ok && current {
		if hadEnabled, _ := state["had_enabled"].(bool); hadEnabled {
			hooks["enabled"] = state["enabled"]
		} else {
			delete(hooks, "enabled")
		}
	}
	_ = os.Remove(statePath)
	return nil
}

func isZCodeHooksEnabled(path string) bool {
	cfg, err := readJSONObjectFile(path)
	if err != nil {
		return false
	}
	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks == nil {
		return false
	}
	enabled, _ := hooks["enabled"].(bool)
	return enabled
}

func installZCodeNativeHook(h HookDef) error {
	cfg, err := readJSONObjectFile(h.NativeConfigPath)
	if err != nil {
		return err
	}
	if _, err := ensureHookRelayScript(h); err != nil {
		return err
	}

	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = make(map[string]interface{})
	}
	if err := captureZCodeHooksEnabledState(h, hooks); err != nil {
		return err
	}
	hooks["enabled"] = true

	events, _ := hooks["events"].(map[string]interface{})
	if events == nil {
		events = make(map[string]interface{})
	}
	for _, eventName := range zcodeHookEvents {
		entries, _ := events[eventName].([]interface{})
		entries = filterZCodeHookEntries(entries)
		commandHook := map[string]interface{}{
			"type":          "command",
			"command":       hookCommand(h, eventName),
			"enabled":       true,
			"async":         true,
			"timeoutMs":     5000,
			"statusMessage": "agent-ebpf: observing ZCode runtime",
		}
		matcher := map[string]interface{}{
			"hooks": []interface{}{commandHook},
		}
		if pattern := zcodeEventMatcher(eventName); pattern != "" {
			matcher["matcher"] = pattern
		}
		events[eventName] = append(entries, matcher)
	}
	hooks["events"] = events
	cfg["hooks"] = hooks
	return writeJSONObjectFile(h.NativeConfigPath, cfg)
}

func uninstallZCodeNativeHook(h HookDef) error {
	cfg, err := readJSONObjectFile(h.NativeConfigPath)
	if err != nil {
		_ = os.Remove(hookRelayScriptPath(h))
		return nil
	}
	hooks, _ := cfg["hooks"].(map[string]interface{})
	if hooks != nil {
		events, _ := hooks["events"].(map[string]interface{})
		if events != nil {
			for _, eventName := range zcodeHookEvents {
				entries, _ := events[eventName].([]interface{})
				filtered := filterZCodeHookEntries(entries)
				if len(filtered) == 0 {
					delete(events, eventName)
				} else {
					events[eventName] = filtered
				}
			}
			hooks["events"] = events
			if err := restoreZCodeHooksEnabledState(h, hooks); err != nil {
				return err
			}
			cfg["hooks"] = hooks
			if err := writeJSONObjectFile(h.NativeConfigPath, cfg); err != nil {
				return err
			}
		}
	}
	_ = os.Remove(hookRelayScriptPath(h))
	return nil
}

func buildZCodeHookRelayScript(h HookDef) string {
	hookSecret := runtimeSettingsStore.HookSecret(h.ID)
	return fmt.Sprintf(`#!/usr/bin/env bash
hook_event="${1:-${AGENT_EBPF_HOOK_EVENT:-}}"
tmp_file="$(mktemp "${TMPDIR:-/tmp}/agent-ebpf-zcode-hook.XXXXXX")" || exit 0
trap 'rm -f "$tmp_file"' EXIT
cat >"$tmp_file"
curl -fsS -X POST '%s' \
  -H 'Content-Type: application/json' \
  -H 'X-Agent-CLI: zcode' \
  -H "X-Agent-Hook-Event: $hook_event" \
  -H "X-Agent-Hook-Parent-PID: $PPID" \
  -H 'X-Agent-Hook-Secret: %s' \
  --data-binary "@$tmp_file" \
  >/dev/null 2>&1 || true
# Empty stdout intentionally keeps ZCode's native permission decision unchanged.
exit 0
`, platform.ResolveHookCallbackURL(), hookSecret)
}
