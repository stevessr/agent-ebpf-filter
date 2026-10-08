package handlers

import (
	"encoding/json"
	"strings"
)

func annotateResponsesMetadata(event *Event, rawBody ...string) {
	if event == nil {
		return
	}
	body := event.Body
	if len(rawBody) > 0 {
		body = rawBody[0]
	}
	if strings.TrimSpace(body) == "" {
		return
	}
	var payload map[string]any
	if json.Unmarshal([]byte(body), &payload) != nil {
		return
	}
	event.ProtocolEvent, _ = payload["type"].(string)
	event.StreamID, _ = payload["stream_id"].(string)
	event.PreviousResponseID, _ = payload["previous_response_id"].(string)
	event.ResponseID, _ = payload["response_id"].(string)
	if response, ok := payload["response"].(map[string]any); ok {
		if event.ResponseID == "" {
			event.ResponseID, _ = response["id"].(string)
		}
		if event.PreviousResponseID == "" {
			event.PreviousResponseID, _ = response["previous_response_id"].(string)
		}
	}
	if event.PreviousResponseID == "" {
		if steer, ok := payload["steer"].(map[string]any); ok {
			event.PreviousResponseID, _ = steer["previous_response_id"].(string)
		}
	}
}

func annotateResponsesContextMetadata(event *Event, rawBody, contentType string) {
	if event == nil || event.Direction != "send" {
		return
	}
	trimmed := strings.TrimSpace(rawBody)
	if trimmed == "" || !looksLikeAgentJSON(contentType, trimmed) {
		return
	}
	metadataBody := sanitizeBody(trimmed, contentType)
	var payload map[string]any
	if json.Unmarshal([]byte(metadataBody), &payload) != nil {
		return
	}

	input := payload["input"]
	if input == nil {
		if response, ok := payload["response"].(map[string]any); ok {
			input = response["input"]
		}
	}
	contextText, itemCount := flattenResponsesContext(input)
	if contextText == "" {
		return
	}
	contextText = sanitizeInlineSecrets(contextText)
	event.ContextDigest = digestPromptText(contextText)
	event.ContextLen = len(contextText)
	event.ContextItems = itemCount
}

func flattenResponsesContext(value any) (string, int) {
	var parts []string
	items := 0

	appendPart := func(role, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		role = strings.TrimSpace(role)
		if role == "" {
			role = "item"
		}
		parts = append(parts, role+"\x1f"+text)
		items++
	}

	switch typed := value.(type) {
	case string:
		appendPart("user", typed)
	case []any:
		for _, raw := range typed {
			switch item := raw.(type) {
			case string:
				appendPart("user", item)
			case map[string]any:
				role, _ := item["role"].(string)
				text := stringifyAgentContent(item["content"])
				if text == "" {
					text = stringifyAgentContent(item["parts"])
				}
				if text == "" {
					text = stringifyResponsesOutputItem(item)
				}
				if role == "" {
					if itemType, _ := item["type"].(string); itemType != "" {
						role = itemType
					}
				}
				appendPart(role, text)
			}
		}
	case map[string]any:
		role, _ := typed["role"].(string)
		text := stringifyAgentContent(typed["content"])
		if text == "" {
			text = stringifyAgentContent(typed["parts"])
		}
		if text == "" {
			text = stringifyResponsesOutputItem(typed)
		}
		if role == "" {
			role, _ = typed["type"].(string)
		}
		appendPart(role, text)
	}
	return strings.Join(parts, "\x1e"), items
}

func extractResponsesAgentText(payload map[string]any, direction string) (string, string) {
	if payload == nil {
		return "", ""
	}
	eventType, _ := payload["type"].(string)

	// WebSocket response.create may be flat (current API) or carry a legacy
	// nested response object. Both forms are accepted by the capture adapter.
	if eventType == "response.create" {
		if response, ok := payload["response"].(map[string]any); ok {
			if text, role := extractResponsesInput(response["input"]); text != "" {
				return text, role
			}
		}
		if text, role := extractResponsesInput(payload["input"]); text != "" {
			return text, role
		}
	}

	// Ordinary POST /v1/responses requests use the same input shape.
	if text, role := extractResponsesInput(payload["input"]); text != "" {
		return text, role
	}

	if direction != "recv" {
		return "", ""
	}
	switch eventType {
	case "response.output_text.delta":
		if delta, ok := payload["delta"].(string); ok && strings.TrimSpace(delta) != "" {
			return delta, "assistant"
		}
	case "response.output_text.done":
		if text, ok := payload["text"].(string); ok && strings.TrimSpace(text) != "" {
			return text, "assistant"
		}
	case "response.content_part.added", "response.content_part.done":
		if text := stringifyAgentContent(payload["part"]); text != "" {
			return text, "assistant"
		}
	case "response.output_item.added", "response.output_item.done":
		if text := stringifyResponsesOutputItem(payload["item"]); text != "" {
			return text, "assistant"
		}
	}
	if response, ok := payload["response"].(map[string]any); ok {
		if text := stringifyResponsesOutput(response["output"]); text != "" {
			return text, "assistant"
		}
		if text, role := extractAgentResponseFromPayload(response); text != "" {
			return text, role
		}
	}
	if text := stringifyResponsesOutput(payload["output"]); text != "" {
		return text, "assistant"
	}
	return "", ""
}

func extractResponsesInput(value any) (string, string) {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return text, "user"
		}
	case []any:
		if text, role := extractAgentMessageFromList(typed, "user", "developer", "system", "tool"); text != "" {
			return text, role
		}
		for i := len(typed) - 1; i >= 0; i-- {
			item, ok := typed[i].(map[string]any)
			if !ok {
				continue
			}
			if output, ok := item["output"].(string); ok && strings.TrimSpace(output) != "" {
				return strings.TrimSpace(output), "tool"
			}
			if text := stringifyResponsesOutputItem(item); text != "" {
				role, _ := item["role"].(string)
				if role == "" {
					role = "user"
				}
				return text, role
			}
		}
	case map[string]any:
		if text := stringifyResponsesOutputItem(typed); text != "" {
			role, _ := typed["role"].(string)
			if role == "" {
				role = "user"
			}
			return text, role
		}
	}
	return "", ""
}

func stringifyResponsesOutput(value any) string {
	items, ok := value.([]any)
	if !ok {
		return stringifyAgentContent(value)
	}
	var parts []string
	for _, item := range items {
		if text := stringifyResponsesOutputItem(item); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func stringifyResponsesOutputItem(value any) string {
	item, ok := value.(map[string]any)
	if !ok {
		return stringifyAgentContent(value)
	}
	if text := stringifyAgentContent(item["content"]); text != "" {
		return text
	}
	for _, key := range []string{"text", "output", "arguments"} {
		if text, ok := item[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}
