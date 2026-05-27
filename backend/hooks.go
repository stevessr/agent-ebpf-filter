package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
	"github.com/pelletier/go-toml/v2"
)

func handleNativeHookEvent(c *gin.Context) {
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(400, gin.H{"error": "invalid json"})
		return
	}

	toolName, _ := payload["tool_name"].(string)
	hookEvent, _ := payload["hook_event_name"].(string)
	toolInput, _ := payload["tool_input"].(map[string]interface{})

	if toolName == "" {
		toolName, _ = payload["tool"].(string)
	}
	if toolName == "" {
		toolName, _ = payload["toolName"].(string)
	}
	if toolInput == nil {
		toolInput, _ = payload["toolInput"].(map[string]interface{})
	}
	if toolInput == nil {
		toolInput, _ = payload["toolArgs"].(map[string]interface{})
	}
	if toolCall, _ := payload["toolCall"].(map[string]interface{}); toolCall != nil {
		if toolName == "" {
			toolName, _ = toolCall["name"].(string)
		}
		if toolInput == nil {
			toolInput, _ = toolCall["args"].(map[string]interface{})
		}
	}
	if hookEvent == "" {
		hookEvent, _ = payload["event"].(string)
	}
	if hookEvent == "" {
		hookEvent, _ = payload["hookEventName"].(string)
	}
	if hookEvent == "" {
		hookEvent = strings.TrimSpace(c.GetHeader("X-Agent-Hook-Event"))
	}

	path := ""
	if toolInput != nil {
		path = extractNativeHookPath(toolInput)
	}
	extraInfo := buildNativeHookExtraInfo(payload, hookEvent, toolName)

	pid, ctx := buildProcessContextFromHookPayload(payload, toolName, path)
	if pid != 0 {
		trackedProcessContexts.Set(pid, ctx)
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
	} else if sourceCLI == "antigravity" || sourceCLI == "agy" || strings.Contains(ua, "antigravity") || strings.Contains(ua, "agy") {
		tag = "Antigravity CLI"
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
		Pid:            pid,
		Type:           "native_hook",
		EventType:      pb.EventType_NATIVE_HOOK,
		Tag:            tag,
		Comm:           fmt.Sprintf("%s:%s", hookEvent, toolName),
		Path:           path,
		ExtraInfo:      extraInfo,
		SchemaVersion:  eventSchemaVersion,
		RootAgentPid:   ctx.RootAgentPid,
		AgentRunId:     ctx.AgentRunID,
		TaskId:         ctx.TaskID,
		ConversationId: ctx.ConversationID,
		TurnId:         ctx.TurnID,
		ToolCallId:     ctx.ToolCallID,
		ToolName:       ctx.ToolName,
		TraceId:        ctx.TraceID,
		SpanId:         ctx.SpanID,
		Decision:       ctx.Decision,
		RiskScore:      ctx.RiskScore,
		ContainerId:    ctx.ContainerID,
		ArgvDigest:     ctx.ArgvDigest,
		Cwd:            ctx.Cwd,
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func extractNativeHookPath(toolInput map[string]interface{}) string {
	for _, key := range []string{
		"command", "Command", "CommandLine", "commandLine",
		"file_path", "filePath", "AbsolutePath", "absolutePath",
		"TargetFile", "targetFile", "DirectoryPath", "directoryPath",
		"SearchPath", "searchPath", "Cwd", "cwd",
	} {
		if value, ok := toolInput[key]; ok {
			if text := interfaceString(value); text != "" {
				return text
			}
		}
	}
	if args, ok := toolInput["arguments"].([]interface{}); ok && len(args) > 0 {
		return fmt.Sprintf("%v", args)
	}
	return ""
}

func buildNativeHookExtraInfo(payload map[string]interface{}, hookEvent, toolName string) string {
	parts := []string{}
	if trimmed := strings.TrimSpace(hookEvent); trimmed != "" {
		parts = append(parts, "hook_event="+sanitizeExtraInfoValue(trimmed))
	}
	if trimmed := strings.TrimSpace(toolName); trimmed != "" {
		parts = append(parts, "tool_name="+sanitizeExtraInfoValue(trimmed))
	}
	if prompt := extractHookPromptText(payload, hookEvent); prompt != "" {
		parts = append(parts,
			"prompt_digest="+digestHookText(prompt),
			fmt.Sprintf("prompt_len=%d", len([]rune(prompt))),
		)
	}
	if response := extractHookResponseText(payload, hookEvent); response != "" {
		parts = append(parts,
			"response_digest="+digestHookText(response),
			fmt.Sprintf("response_len=%d", len([]rune(response))),
		)
	}
	if sessionID := payloadString(payload, "session_id", "sessionId"); sessionID != "" {
		parts = append(parts, "session_id="+sanitizeExtraInfoValue(sessionID))
	}
	return strings.Join(parts, " ")
}

func extractHookPromptText(payload map[string]interface{}, hookEvent string) string {
	if payload == nil {
		return ""
	}
	if prompt := payloadNestedString(payload, "prompt", "initial_prompt", "initialPrompt", "user_prompt", "userPrompt", "input"); prompt != "" {
		return prompt
	}
	lowerEvent := strings.ToLower(strings.TrimSpace(hookEvent))
	if strings.Contains(lowerEvent, "prompt") {
		return payloadNestedString(payload, "message", "text", "content")
	}
	return ""
}

func extractHookResponseText(payload map[string]interface{}, hookEvent string) string {
	if payload == nil {
		return ""
	}
	if response := payloadNestedString(payload, "response", "prompt_response", "promptResponse", "final_response", "finalResponse", "output"); response != "" {
		return response
	}
	lowerEvent := strings.ToLower(strings.TrimSpace(hookEvent))
	if strings.Contains(lowerEvent, "afteragent") || strings.Contains(lowerEvent, "stop") || strings.Contains(lowerEvent, "response") {
		return payloadNestedString(payload, "message", "text", "content")
	}
	return ""
}

func payloadNestedString(payload map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			if text := interfaceString(value); text != "" {
				return text
			}
		}
	}
	for _, nestedKey := range []string{"tool_input", "toolInput", "tool_response", "toolResponse", "tool_output", "toolOutput", "tool_result", "toolResult"} {
		nested, _ := payload[nestedKey].(map[string]interface{})
		if nested == nil {
			continue
		}
		if text := payloadNestedString(nested, keys...); text != "" {
			return text
		}
	}
	return ""
}

func interfaceString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func digestHookText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

func sanitizeExtraInfoValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "\n", "_")
	value = strings.ReplaceAll(value, "\r", "_")
	value = strings.ReplaceAll(value, "\t", "_")
	return value
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
	if err := mkdirAllAsRealUser(filepath.Dir(cfgPath), 0755); err != nil {
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
	return writeFileAsRealUser(cfgPath, out, 0644)
}

func hookRelayScriptPath(h HookDef) string {
	return filepath.Join(filepath.Dir(h.NativeConfigPath), "hooks", hookMarker+"-"+h.ID+".sh")
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
	if err := mkdirAllAsRealUser(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAsRealUser(path, b, 0644)
}

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
