package main

// AI Tool Metadata Enrichment for TLS Events
// 为 TLS 捕获的事件添加 AI 工具元数据标记

import (
	"strings"
)

// AIToolMetadata AI 工具元数据
type AIToolMetadata struct {
	ToolName    string `json:"tool_name"`
	ToolVendor  string `json:"tool_vendor"`
	ToolType    string `json:"tool_type"` // "ai_assistant", "code_completion", "agent"
	APIProvider string `json:"api_provider,omitempty"`
}

// DetectAIToolFromComm 从进程名推断 AI 工具
func DetectAIToolFromComm(comm string) *AIToolMetadata {
	lower := strings.ToLower(comm)

	switch {
	case strings.Contains(lower, "cursor"):
		return &AIToolMetadata{
			ToolName:    "Cursor",
			ToolVendor:  "Cursor",
			ToolType:    "ai_assistant",
			APIProvider: "anthropic",
		}
	case lower == "codex":
		return &AIToolMetadata{
			ToolName:    "Codex",
			ToolVendor:  "OpenAI",
			ToolType:    "ai_assistant",
			APIProvider: "openai",
		}
	}
	return nil
}

// DetectAIToolFromPath 从二进制路径推断 AI 工具
func DetectAIToolFromPath(binPath string) *AIToolMetadata {
	lower := strings.ToLower(binPath)

	if strings.Contains(lower, "@cometix/claude-code") {
		return &AIToolMetadata{
			ToolName:    "Claude Code",
			ToolVendor:  "Anthropic",
			ToolType:    "ai_assistant",
			APIProvider: "anthropic",
		}
	}
	if strings.Contains(lower, "@openai/codex") {
		return &AIToolMetadata{
			ToolName:    "Codex",
			ToolVendor:  "OpenAI",
			ToolType:    "ai_assistant",
			APIProvider: "openai",
		}
	}
	if strings.Contains(lower, "cursor") {
		return &AIToolMetadata{
			ToolName:    "Cursor",
			ToolVendor:  "Cursor",
			ToolType:    "ai_assistant",
			APIProvider: "anthropic",
		}
	}
	if strings.Contains(lower, "github-copilot") {
		return &AIToolMetadata{
			ToolName:    "GitHub Copilot",
			ToolVendor:  "GitHub",
			ToolType:    "code_completion",
			APIProvider: "openai",
		}
	}

	return nil
}

// DetectAIToolFromURL 从 URL 推断 API 提供商
func DetectAIToolFromURL(url, host string) *AIToolMetadata {
	lower := strings.ToLower(host)

	if strings.Contains(lower, "anthropic.com") {
		return &AIToolMetadata{
			APIProvider: "anthropic",
			ToolType:    "ai_assistant",
		}
	}
	if strings.Contains(lower, "openai.com") {
		return &AIToolMetadata{
			APIProvider: "openai",
			ToolType:    "ai_assistant",
		}
	}
	if strings.Contains(lower, "api.openai.com") && strings.Contains(strings.ToLower(url), "/chat/completions") {
		return &AIToolMetadata{
			APIProvider: "openai",
			ToolType:    "ai_assistant",
		}
	}

	return nil
}

// EnrichTLSEventWithAIMetadata 为 TLS 事件添加 AI 工具元数据
func EnrichTLSEventWithAIMetadata(event map[string]interface{}, pid uint32, comm, binPath, url, host string) {
	var meta *AIToolMetadata

	// 优先级 1: 从进程名检测
	if meta == nil && comm != "" {
		meta = DetectAIToolFromComm(comm)
	}

	// 优先级 2: 从二进制路径检测
	if meta == nil && binPath != "" {
		meta = DetectAIToolFromPath(binPath)
	}

	// 优先级 3: 从 URL/Host 检测
	if meta == nil && (url != "" || host != "") {
		meta = DetectAIToolFromURL(url, host)
	}

	// 添加元数据到事件
	if meta != nil {
		if meta.ToolName != "" {
			event["ai_tool_name"] = meta.ToolName
		}
		if meta.ToolVendor != "" {
			event["ai_tool_vendor"] = meta.ToolVendor
		}
		if meta.ToolType != "" {
			event["ai_tool_type"] = meta.ToolType
		}
		if meta.APIProvider != "" {
			event["ai_api_provider"] = meta.APIProvider
		}

		// 设置 runner 标签
		if meta.ToolName != "" {
			event["ai_runner"] = strings.ToLower(strings.ReplaceAll(meta.ToolName, " ", "_"))
		}
	}
}

// Example: 集成到现有的 TLS 事件处理
type TLSPlaintextEventEnriched struct {
	// 原有字段
	Type      string
	PID       uint32
	Comm      string
	URL       string
	Host      string

	// AI 元数据字段
	AIToolName     string `json:"ai_tool_name,omitempty"`
	AIToolVendor   string `json:"ai_tool_vendor,omitempty"`
	AIToolType     string `json:"ai_tool_type,omitempty"`
	AIAPIProvider  string `json:"ai_api_provider,omitempty"`
	AIRunner       string `json:"ai_runner,omitempty"`
}

func main() {
	// 示例使用
	event := make(map[string]interface{})

	// 测试 1: Claude Code
	EnrichTLSEventWithAIMetadata(event, 12345, "node",
		"/home/user/.local/share/pnpm/global/node_modules/@cometix/claude-code/cli.js",
		"/v1/messages", "api.anthropic.com")

	println("Claude Code detection:")
	for k, v := range event {
		if strings.HasPrefix(k, "ai_") {
			println("  ", k, ":", v)
		}
	}

	// 测试 2: Codex
	event = make(map[string]interface{})
	EnrichTLSEventWithAIMetadata(event, 12346, "codex",
		"/home/user/.local/share/pnpm/store/@openai/codex/vendor/x86_64-unknown-linux-musl/bin/codex",
		"/v1/chat/completions", "api.openai.com")

	println("\nCodex detection:")
	for k, v := range event {
		if strings.HasPrefix(k, "ai_") {
			println("  ", k, ":", v)
		}
	}
}
