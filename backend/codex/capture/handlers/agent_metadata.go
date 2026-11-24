package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

func annotateAgentMessage(event *Event) {
	if event == nil {
		return
	}
	annotateAgentMessageFromBody(event, event.Body, event.ContentType)
}

func annotateAgentMessageFromBody(event *Event, rawBody, contentType string) {
	if event == nil || strings.TrimSpace(rawBody) == "" {
		return
	}
	metadataBody := sanitizeBody(rawBody, contentType)
	digest, role, vendor, length := extractAgentMessageMeta(metadataBody, contentType, event.Host, event.URL, event.Direction)
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

func extractAgentMessageMeta(body, contentType, host, rawURL, direction string) (digest, role, vendor string, length int) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", "", "", 0
	}

	if !looksLikeAgentJSON(contentType, trimmed) {
		return digestPromptText(trimmed), "raw", inferVendor(host, rawURL), len(trimmed)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return digestPromptText(trimmed), "raw", inferVendor(host, rawURL), len(trimmed)
	}

	vendor = inferVendor(host, rawURL)
	if extracted, role := extractAgentPromptFromPayload(payload, direction); extracted != "" {
		return digestPromptText(extracted), role, vendor, len(extracted)
	}
	return digestPromptText(trimmed), "raw", vendor, len(trimmed)
}

func digestPromptText(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

func inferVendor(host, rawURL string) string {
	lower := strings.ToLower(host + " " + rawURL)
	switch {
	case strings.Contains(lower, "anthropic") || strings.Contains(lower, "claude"):
		return "anthropic"
	case strings.Contains(lower, "openai"):
		return "openai"
	case strings.Contains(lower, "googleapis") || strings.Contains(lower, "gemini"):
		return "google"
	case strings.Contains(lower, "ollama"):
		return "ollama"
	case strings.Contains(lower, "codex"):
		return "codex"
	default:
		return "unknown"
	}
}

func looksLikeAgentJSON(contentType, body string) bool {
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
	if text, role := extractResponsesAgentText(payload, direction); text != "" {
		return text, role
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
