package app

// AI Tool Detection and Metadata Enrichment
// 为 TLS 事件自动添加 AI 工具识别标签

import (
	"fmt"
	"os"
	"strings"
)

type aiToolMetadata struct {
	ToolName    string
	ToolVendor  string
	ToolType    string
	APIProvider string
}

func detectAIToolFromComm(comm string) *aiToolMetadata {
	lower := strings.ToLower(comm)
	if lower == "codex" {
		return &aiToolMetadata{ToolName: "Codex", ToolVendor: "OpenAI", ToolType: "ai_assistant", APIProvider: "openai"}
	}
	if strings.Contains(lower, "cursor") {
		return &aiToolMetadata{ToolName: "Cursor", ToolVendor: "Cursor", ToolType: "ai_assistant", APIProvider: "anthropic"}
	}
	return nil
}

func detectAIToolFromCmdline(cmdline string) *aiToolMetadata {
	lower := strings.ToLower(cmdline)
	if strings.Contains(lower, "@cometix/claude-code") {
		return &aiToolMetadata{ToolName: "Claude Code", ToolVendor: "Anthropic", ToolType: "ai_assistant", APIProvider: "anthropic"}
	}
	if strings.Contains(lower, "@openai/codex") {
		return &aiToolMetadata{ToolName: "Codex", ToolVendor: "OpenAI", ToolType: "ai_assistant", APIProvider: "openai"}
	}
	if strings.Contains(lower, "cursor") {
		return &aiToolMetadata{ToolName: "Cursor", ToolVendor: "Cursor", ToolType: "ai_assistant", APIProvider: "anthropic"}
	}
	if strings.Contains(lower, "github-copilot") {
		return &aiToolMetadata{ToolName: "GitHub Copilot", ToolVendor: "GitHub", ToolType: "code_completion", APIProvider: "openai"}
	}
	return nil
}

func detectAPIProviderFromHost(host string) string {
	lower := strings.ToLower(host)
	if strings.Contains(lower, "anthropic.com") {
		return "anthropic"
	}
	if strings.Contains(lower, "openai.com") {
		return "openai"
	}
	return ""
}

// enrichTLSEventWithAIMetadata 为 TLS 事件添加 AI 工具标签
// 在 agentSightEventFromTLSPlaintext 中调用
func enrichTLSEventWithAIMetadata(data map[string]any, event TLSPlaintextEvent) {
	var meta *aiToolMetadata

	if event.Comm != "" {
		meta = detectAIToolFromComm(event.Comm)
	}

	if meta == nil && event.PID > 0 {
		cmdlineBytes, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", event.PID))
		if len(cmdlineBytes) > 0 {
			meta = detectAIToolFromCmdline(string(cmdlineBytes))
		}
	}

	if meta != nil {
		if meta.ToolName != "" {
			data["ai_tool"] = meta.ToolName
		}
		if meta.ToolVendor != "" {
			data["ai_vendor"] = meta.ToolVendor
		}
		if meta.APIProvider != "" {
			data["ai_provider"] = meta.APIProvider
		}
	} else if event.Host != "" {
		if provider := detectAPIProviderFromHost(event.Host); provider != "" {
			data["ai_provider"] = provider
		}
	}
}
