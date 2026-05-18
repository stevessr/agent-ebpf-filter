package main

import (
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
