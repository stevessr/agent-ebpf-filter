package main

import (
	"encoding/json"
	"strings"
)

// enrichTLSEventWithAgentContext copies the tracked process context for the
// PID/TGID owning the TLS event into the event itself, allowing downstream
// consumers (frontend, semantic alerting, execution graph) to correlate the
// plaintext traffic back to a specific Agent run, task, or tool call.
func enrichTLSEventWithAgentContext(event *TLSPlaintextEvent) {
	if event == nil {
		return
	}
	ctx, ok := lookupTLSProcessContext(event.PID, event.TGID)
	if !ok {
		return
	}
	if event.RootAgentPID == 0 {
		event.RootAgentPID = ctx.RootAgentPid
	}
	if event.AgentRunID == "" {
		event.AgentRunID = ctx.AgentRunID
	}
	if event.TaskID == "" {
		event.TaskID = ctx.TaskID
	}
	if event.ConversationID == "" {
		event.ConversationID = ctx.ConversationID
	}
	if event.TurnID == "" {
		event.TurnID = ctx.TurnID
	}
	if event.ToolCallID == "" {
		event.ToolCallID = ctx.ToolCallID
	}
	if event.ToolName == "" {
		event.ToolName = ctx.ToolName
	}
	if event.TraceID == "" {
		event.TraceID = ctx.TraceID
	}
	if event.SpanID == "" {
		event.SpanID = ctx.SpanID
	}
}

func lookupTLSProcessContext(pid, tgid uint32) (processContext, bool) {
	if ctx, ok := trackedProcessContexts.Get(pid); ok {
		return ctx, true
	}
	if tgid != 0 && tgid != pid {
		if ctx, ok := trackedProcessContexts.Get(tgid); ok {
			return ctx, true
		}
	}
	return processContext{}, false
}

// annotateTLSAgentMessage populates the prompt digest / role / vendor fields
// for chat-completion-shaped HTTP requests and responses. Only metadata is
// retained; the plaintext body stays inside the in-memory TLS capture store
// (which is bounded and explicitly opt-in via tlsCaptureEnabled).
func annotateTLSAgentMessage(event *TLSPlaintextEvent) {
	if event == nil || event.Body == "" {
		return
	}
	digest, role, vendor, length := extractAgentMessageMeta(event.Body, event.ContentType, event.Host, event.URL, event.Direction)
	if digest == "" {
		return
	}
	event.PromptDigest = digest
	event.MessageRole = role
	if event.Vendor == "" {
		event.Vendor = vendor
	}
	event.PromptLen = length
}

// extractAgentMessageMeta inspects an HTTP body and returns a digest of the
// prompt/response text plus a coarse role label. It recognises OpenAI,
// Anthropic, Google Gemini, Ollama, and generic JSON shapes; if the body
// does not look like a structured agent message but still has content, it
// falls back to digesting the raw body to support local proxy traffic.
func extractAgentMessageMeta(body, contentType, host, url, direction string) (digest, role, vendor string, length int) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", "", "", 0
	}

	if !looksLikeTLSAgentJSON(contentType, trimmed) {
		return digestPromptText(trimmed), "raw", inferTLSVendor(host, url), len(trimmed)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return digestPromptText(trimmed), "raw", inferTLSVendor(host, url), len(trimmed)
	}

	vendor = inferTLSVendor(host, url)
	if extracted, role := extractAgentPromptFromPayload(payload, direction); extracted != "" {
		return digestPromptText(extracted), role, vendor, len(extracted)
	}
	return digestPromptText(trimmed), "raw", vendor, len(trimmed)
}

func looksLikeTLSAgentJSON(contentType, body string) bool {
	lower := strings.ToLower(contentType)
	if strings.Contains(lower, "json") {
		return true
	}
	if body == "" {
		return false
	}
	first := body[0]
	return first == '{' || first == '['
}

func extractAgentPromptFromPayload(payload map[string]any, direction string) (string, string) {
	if payload == nil {
		return "", ""
	}

	if messages, ok := payload["messages"].([]any); ok && len(messages) > 0 {
		if text, role := extractAgentMessageFromList(messages, "user", "system", "tool"); text != "" {
			return text, role
		}
	}

	if contents, ok := payload["contents"].([]any); ok && len(contents) > 0 {
		if text, role := extractAgentMessageFromList(contents, "user", "system", "tool"); text != "" {
			return text, role
		}
	}

	if prompt, ok := payload["prompt"].(string); ok && strings.TrimSpace(prompt) != "" {
		return prompt, "prompt"
	}
	if input, ok := payload["input"].(string); ok && strings.TrimSpace(input) != "" {
		return input, "prompt"
	}

	if direction == "recv" {
		if text, role := extractAgentResponseFromPayload(payload); text != "" {
			return text, role
		}
	}
	return "", ""
}

func extractAgentMessageFromList(items []any, preferRoles ...string) (string, string) {
	preferred := map[string]struct{}{}
	for _, r := range preferRoles {
		preferred[r] = struct{}{}
	}

	var fallbackText, fallbackRole string
	for i := len(items) - 1; i >= 0; i-- {
		item, ok := items[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := item["role"].(string)
		text := stringifyAgentContent(item["content"])
		if text == "" {
			text = stringifyAgentContent(item["parts"])
		}
		if text == "" {
			continue
		}
		if _, want := preferred[role]; want {
			return text, role
		}
		if fallbackText == "" {
			fallbackText = text
			fallbackRole = role
		}
	}
	return fallbackText, fallbackRole
}

func stringifyAgentContent(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		var parts []string
		for _, item := range typed {
			switch entry := item.(type) {
			case string:
				if trimmed := strings.TrimSpace(entry); trimmed != "" {
					parts = append(parts, trimmed)
				}
			case map[string]any:
				if text, ok := entry["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, strings.TrimSpace(text))
					continue
				}
				if input, ok := entry["input"].(string); ok && strings.TrimSpace(input) != "" {
					parts = append(parts, strings.TrimSpace(input))
				}
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func extractAgentResponseFromPayload(payload map[string]any) (string, string) {
	if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if message, ok := choice["message"].(map[string]any); ok {
				if text := stringifyAgentContent(message["content"]); text != "" {
					return text, "assistant"
				}
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				if text := stringifyAgentContent(delta["content"]); text != "" {
					return text, "assistant"
				}
			}
		}
	}
	if content, ok := payload["content"].([]any); ok && len(content) > 0 {
		if text := stringifyAgentContent(content); text != "" {
			return text, "assistant"
		}
	}
	if candidates, ok := payload["candidates"].([]any); ok && len(candidates) > 0 {
		if candidate, ok := candidates[0].(map[string]any); ok {
			if content, ok := candidate["content"].(map[string]any); ok {
				if text := stringifyAgentContent(content["parts"]); text != "" {
					return text, "assistant"
				}
			}
		}
	}
	if response, ok := payload["response"].(string); ok && strings.TrimSpace(response) != "" {
		return response, "assistant"
	}
	return "", ""
}
