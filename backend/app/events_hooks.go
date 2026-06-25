package app

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/pb"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section events_hooks.go ----

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
	if sessionID := events.PayloadString(payload, "session_id", "sessionId"); sessionID != "" {
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
