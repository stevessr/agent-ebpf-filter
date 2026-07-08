package tls

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
	if strings.Contains(lower, "claude") {
		return &aiToolMetadata{ToolName: "Claude Code", ToolVendor: "Anthropic", ToolType: "ai_assistant", APIProvider: "anthropic"}
	}
	if strings.Contains(lower, "codex") {
		return &aiToolMetadata{ToolName: "Codex", ToolVendor: "OpenAI", ToolType: "ai_assistant", APIProvider: "openai"}
	}
	if strings.Contains(lower, "cursor") {
		return &aiToolMetadata{ToolName: "Cursor", ToolVendor: "Cursor", ToolType: "ai_assistant", APIProvider: "anthropic"}
	}
	if strings.Contains(lower, "copilot") {
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
	if strings.Contains(lower, "generativelanguage.googleapis.com") {
		return "google"
	}
	if strings.Contains(lower, "aiplatform.googleapis.com") {
		return "google"
	}
	if strings.Contains(lower, "api.deepseek.com") {
		return "deepseek"
	}
	if strings.Contains(lower, "open.bigmodel.cn") {
		return "zhipu"
	}
	if strings.Contains(lower, "api.z.ai") {
		return "zhipu(overseas)"
	}
	if strings.Contains(lower, "api.moonshot.ai") {
		return "moonshot"
	}
	if strings.Contains(lower, "api.") {
		return "unknown"
	}
	return ""
}

// enrichTLSEventWithAIMetadata 为 TLS 事件添加 AI 工具标签
// 在 agentSightEventFromTLSPlaintext 中调用
func EnrichTLSEventWithAIMetadata(data map[string]any, event TLSPlaintextEvent) {
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

	// Populate AI tool metadata
	if meta != nil {
		if meta.ToolName != "" {
			data["ai_tool"] = meta.ToolName
		}
		if meta.ToolVendor != "" {
			data["ai_vendor"] = meta.ToolVendor
		}
		if meta.ToolType != "" {
			data["ai_tool_type"] = meta.ToolType
		}
		if meta.APIProvider != "" {
			data["ai_provider"] = meta.APIProvider
		}
	} else if event.Host != "" {
		if provider := detectAPIProviderFromHost(event.Host); provider != "" {
			data["ai_provider"] = provider
		}
	}

	// AgentSight-compatible SSL data enrichment
	if body := data["body"]; body != nil {
		if bodyStr, ok := body.(string); ok && bodyStr != "" {
			data["data_type"] = DetectSSLDataType(bodyStr)
		}
	} else if raw, ok := data["data"].(string); ok && raw != "" {
		data["data_type"] = DetectSSLDataType(raw)
	}

	// Populate AgentSight-compatible fields from event struct
	if event.UID > 0 {
		data["uid"] = event.UID
	} else if event.PID > 0 {
		if uid := readProcessUID(event.PID); uid > 0 {
			data["uid"] = uid
		}
	}
	if event.TID > 0 {
		data["tid"] = event.TID
	} else if event.PID > 0 {
		if tid := readProcessTID(event.PID); tid > 0 {
			data["tid"] = tid
		}
	}
	if event.IsHandshake {
		data["is_handshake"] = true
	}
	if event.LatencyMs > 0 {
		data["latency_ms"] = event.LatencyMs
	}
	if event.DeltaNs > 0 {
		data["delta_ns"] = event.DeltaNs
	}
	if event.DataType != "" && data["data_type"] == nil {
		data["data_type"] = event.DataType
	}
}

// isTLSHandshakeFragment detects whether a TLS fragment is likely part of an
// SSL/TLS handshake based on its content type byte (first byte of TLS record).
// Content types: 0x16 = Handshake, 0x14 = ChangeCipherSpec, 0x15 = Alert.
func isTLSHandshakeFragment(fragment CompletedTLSFragment) bool {
	if len(fragment.Payload) == 0 {
		return false
	}
	// TLS record header: content_type (1 byte) | version (2 bytes) | length (2 bytes)
	contentType := fragment.Payload[0]
	return contentType == 0x16 // Handshake
}

// readProcessUID reads the UID of a process from /proc/<pid>/loginuid.
func readProcessUID(pid uint32) uint32 {
	content, err := os.ReadFile(fmt.Sprintf("/proc/%d/loginuid", pid))
	if err != nil {
		return 0
	}
	var uid uint32
	if _, err := fmt.Sscanf(strings.TrimSpace(string(content)), "%d", &uid); err != nil {
		return 0
	}
	return uid
}

// readProcessTID reads the TID (thread group's tgid field from /proc/<pid>/status).
func readProcessTID(pid uint32) uint32 {
	content, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return pid // fall back to pid
	}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "Tgid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				var tgid uint32
				if _, err := fmt.Sscanf(fields[1], "%d", &tgid); err == nil {
					return tgid
				}
			}
			break
		}
	}
	return pid
}
