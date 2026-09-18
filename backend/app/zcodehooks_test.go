package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallZCodeNativeHookLifecycle(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{HookSecrets: map[string]string{"zcode": "zcode-secret"}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	configPath := filepath.Join(t.TempDir(), ".zcode", "cli", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := map[string]interface{}{
		"theme": "dark",
		"hooks": map[string]interface{}{
			"enabled": false,
			"events": map[string]interface{}{
				"PreToolUse": []interface{}{
					map[string]interface{}{
						"matcher": "Write",
						"hooks": []interface{}{
							map[string]interface{}{"type": "process", "command": "echo", "args": []interface{}{"keep-me"}},
						},
					},
				},
			},
		},
	}
	initial, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(configPath, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	h := HookDef{
		ID:               "zcode",
		Name:             "ZCode",
		HookType:         HookTypeNative,
		NativeConfigPath: configPath,
		NativeHookEvent:  "PreToolUse",
		NativeMatcher:    "*",
		ConfigFormat:     ConfigFormatJSON,
	}
	if err := installZCodeNativeHook(h); err != nil {
		t.Fatalf("install ZCode hook: %v", err)
	}
	if !isNativeHookInstalled(h) {
		t.Fatal("installed ZCode hook was not detected")
	}

	cfg, err := readJSONObjectFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg["theme"] != "dark" {
		t.Fatalf("unrelated ZCode config was not preserved: %#v", cfg)
	}
	hooks, _ := cfg["hooks"].(map[string]interface{})
	if enabled, _ := hooks["enabled"].(bool); !enabled {
		t.Fatalf("hooks.enabled was not enabled: %#v", hooks)
	}
	events, _ := hooks["events"].(map[string]interface{})
	if len(events) < len(zcodeHookEvents) {
		t.Fatalf("missing ZCode lifecycle events: %#v", events)
	}
	pre, _ := events["PreToolUse"].([]interface{})
	if len(pre) != 2 {
		t.Fatalf("existing PreToolUse hook was not preserved: %#v", pre)
	}
	managed, _ := pre[1].(map[string]interface{})
	if got, _ := managed["matcher"].(string); got != "*" {
		t.Fatalf("unexpected managed matcher: %#v", managed)
	}
	managedHooks, _ := managed["hooks"].([]interface{})
	if len(managedHooks) != 1 {
		t.Fatalf("unexpected managed hook list: %#v", managed)
	}
	commandHook, _ := managedHooks[0].(map[string]interface{})
	if got, _ := commandHook["type"].(string); got != "command" {
		t.Fatalf("ZCode async relay must use command executor, got %#v", commandHook)
	}
	if async, _ := commandHook["async"].(bool); !async {
		t.Fatalf("ZCode telemetry relay must be async: %#v", commandHook)
	}
	command, _ := commandHook["command"].(string)
	if !strings.Contains(command, hookMarker+"-zcode.sh") || !strings.Contains(command, "PreToolUse") {
		t.Fatalf("unexpected managed command: %q", command)
	}

	relayPath := hookRelayScriptPath(h)
	relay, err := os.ReadFile(relayPath)
	if err != nil {
		t.Fatalf("read ZCode relay: %v", err)
	}
	relayText := string(relay)
	for _, want := range []string{"X-Agent-CLI: zcode", "X-Agent-Hook-Parent-PID: $PPID", "X-Agent-Hook-Secret: zcode-secret"} {
		if !strings.Contains(relayText, want) {
			t.Fatalf("ZCode relay missing %q:\n%s", want, relayText)
		}
	}
	if output, err := exec.Command("bash", "-n", relayPath).CombinedOutput(); err != nil {
		t.Fatalf("generated ZCode relay has invalid shell syntax: %v (%s)", err, output)
	}

	if err := uninstallZCodeNativeHook(h); err != nil {
		t.Fatalf("uninstall ZCode hook: %v", err)
	}
	if isNativeHookInstalled(h) {
		t.Fatal("uninstalled ZCode hook still detected")
	}
	cfg, err = readJSONObjectFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	hooks, _ = cfg["hooks"].(map[string]interface{})
	events, _ = hooks["events"].(map[string]interface{})
	pre, _ = events["PreToolUse"].([]interface{})
	if len(pre) != 1 {
		t.Fatalf("uninstall removed user hooks or left managed hook: %#v", pre)
	}
	if cfg["theme"] != "dark" {
		t.Fatalf("uninstall modified unrelated ZCode config: %#v", cfg)
	}
	if _, err := os.Stat(relayPath); !os.IsNotExist(err) {
		t.Fatalf("ZCode relay should be removed, stat err=%v", err)
	}
}

func TestZCodePayloadShapeFeedsPathAndContext(t *testing.T) {
	payload := map[string]interface{}{
		"session_id":      "session-zcode",
		"cwd":             "/workspace/project",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Write",
		"tool_use_id":     "tool-zcode-42",
		"tool_input": map[string]interface{}{
			"file_path": "src/main.go",
		},
	}
	input, _ := payload["tool_input"].(map[string]interface{})
	path := extractNativeHookPath(input)
	if path != "src/main.go" {
		t.Fatalf("unexpected path: %q", path)
	}
	_, ctx := buildProcessContextFromHookPayload(payload, "Write", path)
	if ctx.ToolName != "Write" || ctx.ToolCallID != "tool-zcode-42" || ctx.Cwd != "/workspace/project" {
		t.Fatalf("unexpected ZCode context: %#v", ctx)
	}
}

func TestBuildNativeHookExtraInfoHashesZCodeStopMessage(t *testing.T) {
	payload := map[string]interface{}{
		"session_id":             "session-zcode",
		"permission_mode":        "default",
		"last_assistant_message": "sensitive final answer",
	}
	extra := buildNativeHookExtraInfo(payload, "Stop", "")
	if !strings.Contains(extra, "response_digest=sha256:") || !strings.Contains(extra, "response_len=") || !strings.Contains(extra, "permission_mode=default") {
		t.Fatalf("missing safe response metadata: %q", extra)
	}
	if strings.Contains(extra, "sensitive final answer") {
		t.Fatalf("raw ZCode response leaked into metadata: %q", extra)
	}
}
