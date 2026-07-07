package tls

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ---- moved from backend/zz_merged_backend.go section agentstreamtls.go ----

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

func lookupTLSProcessContext(pid, tgid uint32) (ProcessContext, bool) {
	if ctx, ok := deps.TrackedProcessContexts.Get(pid); ok {
		return ctx, true
	}
	if tgid != 0 && tgid != pid {
		if ctx, ok := deps.TrackedProcessContexts.Get(tgid); ok {
			return ctx, true
		}
	}
	return ProcessContext{}, false
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
					continue
				}
				if ph, ok := summarizeAgentImageBlock(entry); ok && ph != "" {
					parts = append(parts, ph)
				}
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
		if ph, ok := summarizeAgentImageBlock(typed); ok && ph != "" {
			return ph
		}
	}
	return ""
}

// summarizeAgentImageBlock produces a compact, human-readable placeholder for
// an Anthropic `image` or OpenAI `image_url` content block. The placeholder is
// folded into the prompt digest so loop detection sees image-bearing prompts
// (a prompt with and without an image are different prompts). The full base64
// payload is intentionally NOT hashed — it is large and partially captured, so
// including it would flood the digest window and mask repetition.
//
// Recognised shapes:
//   - Anthropic: {"type":"image","source":{"type":"base64","media_type":"image/png","data":"..."}}
//   - OpenAI:    {"type":"image_url","image_url":{"url":"data:image/png;base64,..." | "https://..."}}
//
// Folded sentinels produced by foldTLSImageBase64 (prefix __IMAGE_FOLDED__:) on
// the body path are also recognised here so the digest is stable regardless of
// where folding happened.
func summarizeAgentImageBlock(entry map[string]any) (string, bool) {
	t, _ := entry["type"].(string)
	switch t {
	case "image":
		media := ""
		var dataLen int
		if src, ok := entry["source"].(map[string]any); ok {
			if mt, ok := src["media_type"].(string); ok {
				media = mt
			}
			if data, ok := src["data"].(string); ok {
				dataLen = len(data)
			}
		}
		return imageBlockPlaceholder("anthropic-image", media, dataLen), true
	case "image_url":
		rawURL := ""
		if iu, ok := entry["image_url"].(map[string]any); ok {
			if u, ok := iu["url"].(string); ok {
				rawURL = u
			}
		}
		return summarizeAgentImageURL(rawURL), true
	}
	return "", false
}

// summarizeAgentImageURL converts an OpenAI image_url.url value into a compact
// placeholder. HTTP(S) URLs are hashed (small, stable) into the digest;
// `data:` URIs contribute only their media type and data length so that a
// folded/repeated image is distinguishable but does not flood the digest.
func summarizeAgentImageURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if strings.HasPrefix(rawURL, "data:") {
		media, dataLen := parseAgentDataURI(rawURL)
		return imageBlockPlaceholder("openai-data-image", media, dataLen)
	}
	// Already folded sentinel from foldTLSDataURI — keep as-is to preserve the
	// approx byte count that distinguishes distinct images.
	if strings.HasPrefix(rawURL, tlsImageFoldedPrefix) {
		return rawURL
	}
	return "[image_url " + rawURL + "]"
}

// parseAgentDataURI extracts (mediaType, base64Length) from a data: URI; it
// tolerates a folded sentinel value too.
func parseAgentDataURI(rawURL string) (string, int) {
	if strings.HasPrefix(rawURL, tlsImageFoldedPrefix) {
		rest := strings.TrimPrefix(rawURL, tlsImageFoldedPrefix)
		parts := strings.SplitN(rest, ":", 2)
		media := parts[0]
		size := 0
		if len(parts) == 2 {
			fmt.Sscanf(parts[1], "%d", &size)
		}
		return media, size
	}
	const prefix = "data:"
	if !strings.HasPrefix(rawURL, prefix) {
		return "", 0
	}
	comma := strings.Index(rawURL, ",")
	if comma < 0 {
		return "", 0
	}
	meta := rawURL[len(prefix):comma]
	media := strings.SplitN(meta, ";", 2)[0]
	return media, len(rawURL) - comma - 1
}

// imageBlockPlaceholder builds a digest-friendly placeholder describing an
// embedded image without including its payload.
func imageBlockPlaceholder(kind, mediaType string, base64Len int) string {
	if mediaType == "" {
		mediaType = "image"
	}
	return fmt.Sprintf("[%s %s ~%dB]", kind, mediaType, base64Len*3/4)
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
