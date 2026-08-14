package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- moved from backend/zz_merged_backend_test.go section hooks_test.go ----

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

func TestInstallTypeScriptNativeHookLifecycle(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{HookSecrets: map[string]string{"pi": "pi-secret"}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	observed := make(chan struct {
		cli, secret, event, body string
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		observed <- struct {
			cli, secret, event, body string
		}{
			cli: r.Header.Get("X-Agent-CLI"), secret: r.Header.Get("X-Agent-Hook-Secret"),
			event: r.Header.Get("X-Agent-Hook-Event"), body: string(body),
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	t.Setenv("AGENT_HOOK_ENDPOINT", server.URL)

	h := HookDef{
		ID:               "pi",
		NativeConfigPath: filepath.Join(t.TempDir(), ".pi", "agent", "extensions", hookMarker+"-pi.ts"),
		ConfigFormat:     ConfigFormatTypeScript,
	}
	if err := installTypeScriptNativeHook(h); err != nil {
		t.Fatalf("install Pi extension: %v", err)
	}
	extension, err := os.ReadFile(h.NativeConfigPath)
	if err != nil {
		t.Fatalf("read Pi extension: %v", err)
	}
	if !strings.Contains(string(extension), "agent-ebpf-hook-active") || !strings.Contains(string(extension), "pid: process.pid") || !strings.Contains(string(extension), "session_start") || !strings.Contains(string(extension), "tool_call") || !strings.Contains(string(extension), "tool_result") {
		t.Fatalf("extension is missing marker, PID, or event registrations: %s", extension)
	}
	if strings.Contains(string(extension), "X-Agent-Hook-Secret") {
		t.Fatalf("extension must not embed relay authentication headers")
	}
	if _, err := os.Stat(hookRelayScriptPath(h)); err != nil {
		t.Fatalf("relay script was not written: %v", err)
	}
	relayCheck := exec.Command("bash", "-n", hookRelayScriptPath(h))
	if output, err := relayCheck.CombinedOutput(); err != nil {
		t.Fatalf("generated Pi relay has invalid shell syntax: %v (%s)", err, output)
	}
	relayBytes, err := os.ReadFile(hookRelayScriptPath(h))
	if err != nil {
		t.Fatalf("read Pi relay: %v", err)
	}
	relay := string(relayBytes)
	if !strings.Contains(relay, "X-Agent-CLI: pi") || !strings.Contains(relay, "X-Agent-Hook-Secret: pi-secret") {
		t.Fatalf("Pi relay missing authenticated headers: %s", relay)
	}
	relayRun := exec.Command("bash", hookRelayScriptPath(h), "tool_call")
	relayRun.Stdin = strings.NewReader(`{"hook_event_name":"tool_call","tool_name":"bash"}`)
	if output, err := relayRun.CombinedOutput(); err != nil {
		t.Fatalf("generated Pi relay failed: %v (%s)", err, output)
	}
	select {
	case request := <-observed:
		if request.cli != "pi" || request.secret != "pi-secret" || request.event != "tool_call" {
			t.Fatalf("unexpected relay headers: %#v", request)
		}
		if !strings.Contains(request.body, `"tool_name":"bash"`) {
			t.Fatalf("unexpected relay body: %s", request.body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay request was not received")
	}

	if err := uninstallTypeScriptNativeHook(h); err != nil {
		t.Fatalf("uninstall Pi extension: %v", err)
	}
	if isNativeHookInstalled(h) {
		t.Fatalf("uninstalled Pi extension still detected")
	}
	if _, err := os.Stat(h.NativeConfigPath); !os.IsNotExist(err) {
		t.Fatalf("Pi extension should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(hookRelayScriptPath(h)); !os.IsNotExist(err) {
		t.Fatalf("Pi relay should be removed, stat err=%v", err)
	}
}

func TestResolvePiAndOmpAgentDirs(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_DIR", "")
	if got, want := resolvePiAgentDir("/home/test"), filepath.Join("/home/test", ".pi", "agent"); got != want {
		t.Fatalf("default Pi agent dir = %q, want %q", got, want)
	}
	t.Setenv("PI_CODING_AGENT_DIR", "~/.pi-custom/agent")
	if got, want := resolvePiAgentDir("/home/test"), filepath.Join("/home/test", ".pi-custom", "agent"); got != want {
		t.Fatalf("configured Pi agent dir = %q, want %q", got, want)
	}

	t.Setenv("PI_CONFIG_DIR", ".omp-custom")
	t.Setenv("OMP_PROFILE", "review")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	if got, want := resolveOmpAgentDir("/home/test"), filepath.Join("/home/test", ".omp-custom", "profiles", "review", "agent"); got != want {
		t.Fatalf("profile OMP agent dir = %q, want %q", got, want)
	}
	t.Setenv("OMP_PROFILE", "../escape")
	if got, want := resolveOmpAgentDir("/home/test"), filepath.Join("/home/test", ".omp-custom", "agent"); got != want {
		t.Fatalf("invalid profile OMP agent dir = %q, want %q", got, want)
	}
	t.Setenv("OMP_PROFILE", "default")
	if got, want := resolveOmpAgentDir("/home/test"), filepath.Join("/home/test", ".omp-custom", "agent"); got != want {
		t.Fatalf("default profile OMP agent dir = %q, want %q", got, want)
	}
	t.Setenv("PI_CODING_AGENT_DIR", "~/.omp-custom-agent")
	if got, want := resolveOmpAgentDir("/home/test"), filepath.Join("/home/test", ".omp-custom-agent"); got != want {
		t.Fatalf("default OMP agent override = %q, want %q", got, want)
	}
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("OMP_PROFILE", "")
	t.Setenv("PI_PROFILE", "legacy")
	if got, want := resolveOmpAgentDir("/home/test"), filepath.Join("/home/test", ".omp-custom", "agent"); got != want {
		t.Fatalf("explicitly empty OMP_PROFILE should select default, got %q want %q", got, want)
	}
	_ = os.Unsetenv("OMP_PROFILE")
	if got, want := resolveOmpAgentDir("/home/test"), filepath.Join("/home/test", ".omp-custom", "profiles", "legacy", "agent"); got != want {
		t.Fatalf("legacy PI_PROFILE agent dir = %q, want %q", got, want)
	}
}

func TestInstallOmpTypeScriptNativeHookLifecycle(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{HookSecrets: map[string]string{"omp": "omp-secret"}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	h := HookDef{
		ID:               "omp",
		NativeConfigPath: filepath.Join(t.TempDir(), ".omp", "profiles", "review", "agent", "extensions", hookMarker+"-omp.ts"),
		ConfigFormat:     ConfigFormatTypeScript,
	}
	if err := installTypeScriptNativeHook(h); err != nil {
		t.Fatalf("install OMP extension: %v", err)
	}
	extension, err := os.ReadFile(h.NativeConfigPath)
	if err != nil {
		t.Fatalf("read OMP extension: %v", err)
	}
	if !strings.Contains(string(extension), "export default function register") || !strings.Contains(string(extension), "session_start") || !strings.Contains(string(extension), "tool_call") || !strings.Contains(string(extension), "tool_result") {
		t.Fatalf("OMP extension is missing event registrations: %s", extension)
	}
	if strings.Contains(string(extension), "X-Agent-Hook-Secret") {
		t.Fatalf("OMP extension must not embed relay authentication headers")
	}
	if !isNativeHookInstalled(h) {
		t.Fatalf("installed OMP extension was not detected")
	}
	if _, err := os.Stat(hookRelayScriptPath(h)); err != nil {
		t.Fatalf("OMP relay script was not written: %v", err)
	}
	relayBytes, err := os.ReadFile(hookRelayScriptPath(h))
	if err != nil {
		t.Fatalf("read OMP relay: %v", err)
	}
	if !strings.Contains(string(relayBytes), "X-Agent-CLI: omp") || !strings.Contains(string(relayBytes), "X-Agent-Hook-Secret: omp-secret") {
		t.Fatalf("OMP relay missing authenticated headers: %s", relayBytes)
	}
	if err := uninstallTypeScriptNativeHook(h); err != nil {
		t.Fatalf("uninstall OMP extension: %v", err)
	}
	if _, err := os.Stat(h.NativeConfigPath); !os.IsNotExist(err) {
		t.Fatalf("OMP extension should be removed, stat err=%v", err)
	}
}

func TestPiAndOmpPayloadFeedsProcessContext(t *testing.T) {
	payload := map[string]interface{}{
		"hook_event_name": "tool_call",
		"session_id":      "session-42",
		"cwd":             "/workspace/project",
		"tool_name":       "bash",
		"tool_call_id":    "call-42",
		"tool_input": map[string]interface{}{
			"command": "npm test",
		},
	}
	toolInput, _ := payload["tool_input"].(map[string]interface{})
	path := extractNativeHookPath(toolInput)
	if path != "npm test" {
		t.Fatalf("unexpected Pi/OMP command path: %q", path)
	}
	_, ctx := buildProcessContextFromHookPayload(payload, "bash", path)
	if ctx.ToolName != "bash" || ctx.ToolCallID != "call-42" || ctx.Cwd != "/workspace/project" {
		t.Fatalf("unexpected Pi/OMP context: %#v", ctx)
	}
	extra := buildNativeHookExtraInfo(payload, "tool_call", "bash")
	if strings.Contains(extra, "npm test") {
		t.Fatalf("native hook metadata leaked raw command: %q", extra)
	}
}
