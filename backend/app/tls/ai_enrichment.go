package tls

// AI Tool Detection and Metadata Enrichment
// 为 TLS 事件自动添加 AI 工具识别标签

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	tlsProcessEnrichmentCacheTTL = 30 * time.Second
	tlsProcessEnrichmentCacheMax = 2048
)

type aiToolMetadata struct {
	ToolName    string
	ToolVendor  string
	ToolType    string
	APIProvider string
}

type tlsProcessEnrichmentCacheEntry struct {
	expiresAt time.Time
	tool      *aiToolMetadata
	uid       uint32
	tid       uint32
}

var tlsProcessEnrichmentCache = struct {
	sync.Mutex
	items map[uint32]tlsProcessEnrichmentCacheEntry
}{items: make(map[uint32]tlsProcessEnrichmentCacheEntry)}

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

func cachedTLSProcessEnrichment(pid uint32) tlsProcessEnrichmentCacheEntry {
	if pid == 0 {
		return tlsProcessEnrichmentCacheEntry{}
	}
	now := time.Now()

	tlsProcessEnrichmentCache.Lock()
	if cached, ok := tlsProcessEnrichmentCache.items[pid]; ok {
		if now.Before(cached.expiresAt) {
			tlsProcessEnrichmentCache.Unlock()
			return cached
		}
		delete(tlsProcessEnrichmentCache.items, pid)
	}
	tlsProcessEnrichmentCache.Unlock()

	// Perform procfs I/O outside the cache lock. A burst of simultaneous first
	// events may duplicate one read, but it never stalls enrichment for every
	// other PID behind filesystem access.
	cmdlineBytes, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	entry := tlsProcessEnrichmentCacheEntry{
		expiresAt: now.Add(tlsProcessEnrichmentCacheTTL),
		uid:       readProcessUIDUncached(pid),
		tid:       readProcessTIDUncached(pid),
	}
	if len(cmdlineBytes) > 0 {
		entry.tool = detectAIToolFromCmdline(string(cmdlineBytes))
	}

	tlsProcessEnrichmentCache.Lock()
	// Prefer a fresh value inserted by another goroutine while procfs was read.
	if cached, ok := tlsProcessEnrichmentCache.items[pid]; ok && now.Before(cached.expiresAt) {
		tlsProcessEnrichmentCache.Unlock()
		return cached
	}
	if _, exists := tlsProcessEnrichmentCache.items[pid]; !exists && len(tlsProcessEnrichmentCache.items) >= tlsProcessEnrichmentCacheMax {
		evictOldestTLSProcessEnrichmentLocked()
	}
	tlsProcessEnrichmentCache.items[pid] = entry
	tlsProcessEnrichmentCache.Unlock()
	return entry
}

func evictOldestTLSProcessEnrichmentLocked() {
	var oldestPID uint32
	var oldestExpiry time.Time
	for pid, entry := range tlsProcessEnrichmentCache.items {
		if oldestExpiry.IsZero() || entry.expiresAt.Before(oldestExpiry) {
			oldestPID = pid
			oldestExpiry = entry.expiresAt
		}
	}
	if !oldestExpiry.IsZero() {
		delete(tlsProcessEnrichmentCache.items, oldestPID)
	}
}

// EnrichTLSEventWithAIMetadata adds AI-tool and protocol metadata to the
// AgentSight fast-path map without introducing another JSON round trip.
func EnrichTLSEventWithAIMetadata(data map[string]any, event TLSPlaintextEvent) {
	if strings.HasPrefix(event.Type, "http2_") {
		data["protocol"] = "http2"
		data["http_version"] = "2"
		data["event_type"] = "HTTP2_MESSAGE"
		if event.HTTP2StreamID != 0 {
			data["http2_stream_id"] = event.HTTP2StreamID
		}
		if event.HTTP2FrameType != "" {
			data["http2_frame_type"] = event.HTTP2FrameType
		}
		if event.HTTP2Flags != 0 {
			data["http2_flags"] = event.HTTP2Flags
		}
		if event.HTTP2PromisedStreamID != 0 {
			data["http2_promised_stream_id"] = event.HTTP2PromisedStreamID
		}
		if event.RawHexDump == "" {
			data["raw_available"] = false
		}
	}

	var process tlsProcessEnrichmentCacheEntry
	if event.PID > 0 {
		process = cachedTLSProcessEnrichment(event.PID)
	}

	meta := detectAIToolFromComm(event.Comm)
	if meta == nil {
		meta = process.tool
	}

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

	if body := data["body"]; body != nil {
		if bodyStr, ok := body.(string); ok && bodyStr != "" {
			data["data_type"] = DetectSSLDataType(bodyStr)
		}
	} else if raw, ok := data["data"].(string); ok && raw != "" {
		data["data_type"] = DetectSSLDataType(raw)
	}

	if event.StatusCode > 0 {
		data["status"] = event.StatusCode
	}

	if event.UID > 0 {
		data["uid"] = event.UID
	} else if process.uid > 0 {
		data["uid"] = process.uid
	}
	if event.TID > 0 {
		data["tid"] = event.TID
	} else if process.tid > 0 {
		data["tid"] = process.tid
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

func isTLSHandshakeFragment(fragment CompletedTLSFragment) bool {
	if len(fragment.Payload) == 0 {
		return false
	}
	contentType := fragment.Payload[0]
	return contentType == 0x16
}

// readProcessUID is cached because it is called from the TLS parse hot path.
func readProcessUID(pid uint32) uint32 {
	return cachedTLSProcessEnrichment(pid).uid
}

func readProcessUIDUncached(pid uint32) uint32 {
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

// readProcessTID is cached for AgentSight enrichment. The fallback matches the
// historical behavior and uses the supplied PID when /proc/status disappears.
func readProcessTID(pid uint32) uint32 {
	return cachedTLSProcessEnrichment(pid).tid
}

func readProcessTIDUncached(pid uint32) uint32 {
	content, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return pid
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
