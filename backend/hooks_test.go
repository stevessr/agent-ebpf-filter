package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildNativeHookExtraInfoRecordsSafePromptMetadata(t *testing.T) {
	payload := map[string]interface{}{
		"prompt":     "please inspect SECRET_TOKEN=abc123",
		"session_id": "session-1",
	}

	extra := buildNativeHookExtraInfo(payload, "UserPromptSubmit", "chat")

	if !strings.Contains(extra, "hook_event=UserPromptSubmit") {
		t.Fatalf("extra info missing hook event: %q", extra)
	}
	if !strings.Contains(extra, "prompt_digest=sha256:") || !strings.Contains(extra, "prompt_len=") {
		t.Fatalf("extra info missing prompt metadata: %q", extra)
	}
	if strings.Contains(extra, "SECRET_TOKEN") || strings.Contains(extra, "abc123") {
		t.Fatalf("extra info leaked raw prompt content: %q", extra)
	}
}

func TestBuildNativeHookExtraInfoFindsNestedResponseMetadata(t *testing.T) {
	payload := map[string]interface{}{
		"tool_response": map[string]interface{}{
			"response": "final answer text",
		},
	}

	extra := buildNativeHookExtraInfo(payload, "AfterAgent", "chat")

	if !strings.Contains(extra, "response_digest=sha256:") || !strings.Contains(extra, "response_len=17") {
		t.Fatalf("extra info missing nested response metadata: %q", extra)
	}
	if strings.Contains(extra, "final answer text") {
		t.Fatalf("extra info leaked raw response content: %q", extra)
	}
}

func TestAntigravityRelayScriptReturnsCLIRequiredJSON(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{HookSecrets: map[string]string{"antigravity": "secret"}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	script := buildHookRelayScript(HookDef{ID: "antigravity"})

	for _, want := range []string{
		"X-Agent-CLI: antigravity",
		"X-Agent-Hook-Event: $hook_event",
		`PreToolUse)`,
		`"decision":"allow"`,
		`"injectSteps":[]`,
		`"terminationBehavior":""`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("antigravity relay script missing %q:\n%s", want, script)
		}
	}
}

func TestInstallAntigravityNativeHookWritesPluginHooksJSON(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{HookSecrets: map[string]string{"antigravity": "secret"}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	pluginDir := filepath.Join(t.TempDir(), ".gemini", "antigravity-cli", "plugins", hookMarker)
	h := HookDef{
		ID:               "antigravity",
		NativeConfigPath: filepath.Join(pluginDir, "hooks.json"),
		NativeHookEvent:  "PreToolUse",
		NativeMatcher:    "*",
		ConfigFormat:     ConfigFormatJSON,
	}

	if err := installAntigravityNativeHook(h); err != nil {
		t.Fatalf("install antigravity hook: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.json")); err != nil {
		t.Fatalf("plugin manifest was not written: %v", err)
	}
	if _, err := os.Stat(hookRelayScriptPath(h)); err != nil {
		t.Fatalf("relay script was not written: %v", err)
	}
	if !isNativeHookInstalled(h) {
		t.Fatalf("installed antigravity hook was not detected")
	}

	var cfg map[string]interface{}
	b, err := os.ReadFile(h.NativeConfigPath)
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("parse hooks.json: %v", err)
	}
	hookDef, _ := cfg[hookMarker].(map[string]interface{})
	if hookDef == nil {
		t.Fatalf("hooks.json missing %s definition: %#v", hookMarker, cfg)
	}
	entries, _ := hookDef["PreToolUse"].([]interface{})
	if len(entries) != 1 {
		t.Fatalf("unexpected PreToolUse entries: %#v", hookDef["PreToolUse"])
	}
	matcher, _ := entries[0].(map[string]interface{})
	if got, _ := matcher["matcher"].(string); got != "*" {
		t.Fatalf("unexpected matcher: %#v", matcher)
	}
	hooks, _ := matcher["hooks"].([]interface{})
	if len(hooks) != 1 {
		t.Fatalf("unexpected command hooks: %#v", matcher["hooks"])
	}
	commandHook, _ := hooks[0].(map[string]interface{})
	command, _ := commandHook["command"].(string)
	if !strings.Contains(command, hookMarker+"-antigravity.sh") || !strings.Contains(command, "PreToolUse") {
		t.Fatalf("command does not call event-aware relay script: %q", command)
	}
	if got, _ := commandHook["timeout"].(float64); got != 5 {
		t.Fatalf("antigravity timeout should be seconds=5, got %#v", commandHook["timeout"])
	}

	if err := uninstallAntigravityNativeHook(h); err != nil {
		t.Fatalf("uninstall antigravity hook: %v", err)
	}
	if isNativeHookInstalled(h) {
		t.Fatalf("uninstalled antigravity hook still detected")
	}
	if _, err := os.Stat(hookRelayScriptPath(h)); !os.IsNotExist(err) {
		t.Fatalf("relay script should be removed on uninstall, stat err=%v", err)
	}
}

func TestAntigravityPayloadShapeFeedsPathAndContext(t *testing.T) {
	payload := map[string]interface{}{
		"conversationId": "conv-1",
		"toolCall": map[string]interface{}{
			"name": "run_command",
			"args": map[string]interface{}{
				"CommandLine": "npm test",
				"Cwd":         "/workspace/project",
			},
		},
	}
	toolCall, _ := payload["toolCall"].(map[string]interface{})
	toolInput, _ := toolCall["args"].(map[string]interface{})
	path := extractNativeHookPath(toolInput)
	if path != "npm test" {
		t.Fatalf("unexpected antigravity command path: %q", path)
	}
	_, ctx := buildProcessContextFromHookPayload(payload, "", path)
	if ctx.ToolName != "run_command" || ctx.ConversationID != "conv-1" || ctx.Cwd != "/workspace/project" {
		t.Fatalf("unexpected antigravity context: %#v", ctx)
	}
}
