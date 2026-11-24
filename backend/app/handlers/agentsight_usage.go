package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"agent-ebpf-filter/app/tls"

	"github.com/gin-gonic/gin"
)

// AgentSightUsageSummary is a bounded, metadata-only report. It never exports
// prompt bodies, TLS plaintext, filenames, or request headers.
type AgentSightUsageSummary struct {
	EventCount          int                      `json:"event_count"`
	UnattributedEvents  int                      `json:"unattributed_events"`
	UsageEvents         int                      `json:"usage_events"`
	InputTokens         uint64                   `json:"input_tokens"`
	OutputTokens        uint64                   `json:"output_tokens"`
	CacheReadTokens     uint64                   `json:"cache_read_tokens"`
	CacheCreationTokens uint64                   `json:"cache_creation_tokens"`
	Sessions            []AgentSightSessionUsage `json:"sessions"`
	Models              []AgentSightModelUsage   `json:"models"`
	Coverage            string                   `json:"coverage"`
}

type AgentSightSessionUsage struct {
	ID                  string `json:"id"`
	EventCount          int    `json:"event_count"`
	UsageEvents         int    `json:"usage_events"`
	InputTokens         uint64 `json:"input_tokens"`
	OutputTokens        uint64 `json:"output_tokens"`
	CacheReadTokens     uint64 `json:"cache_read_tokens"`
	CacheCreationTokens uint64 `json:"cache_creation_tokens"`
	LastEventTimestamp  int64  `json:"last_event_timestamp"`
}

type AgentSightModelUsage struct {
	Model               string `json:"model"`
	UsageEvents         int    `json:"usage_events"`
	InputTokens         uint64 `json:"input_tokens"`
	OutputTokens        uint64 `json:"output_tokens"`
	CacheReadTokens     uint64 `json:"cache_read_tokens"`
	CacheCreationTokens uint64 `json:"cache_creation_tokens"`
}

type agentSightTokenUsage struct {
	Input, Output, CacheRead, CacheCreation uint64
}

// HandleAgentSightUsageReport exposes an AgentSight-style token/session report
// from the already-retained, already-redacted events; it does not start probes.
func HandleAgentSightUsageReport(tlsStore *tls.TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := agentSightQueryFromRequest(c)
		events, source, err := collectAgentSightEventsForQuery(c.Request.Context(), query, tlsStore)
		if err != nil {
			if errors.Is(err, context.Canceled) || c.Request.Context().Err() != nil {
				return
			}
			c.JSON(agentSightErrorStatus(err), gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"source":  source,
			"limit":   query.Limit,
			"summary": BuildAgentSightUsageSummary(events, strings.TrimSpace(c.Query("session"))),
		})
	}
}

// BuildAgentSightUsageSummary counts only explicit model usage metadata.
// A zero token count with no usage events is *unknown*, not free inference.
func BuildAgentSightUsageSummary(events []AgentSightExportEvent, sessionFilter string) AgentSightUsageSummary {
	summary := AgentSightUsageSummary{
		Sessions: []AgentSightSessionUsage{},
		Models:   []AgentSightModelUsage{},
		Coverage: "explicit_usage_only",
	}
	sessions := map[string]*AgentSightSessionUsage{}
	models := map[string]*AgentSightModelUsage{}
	seenCalls := make(map[string]struct{}, len(events))
	for _, event := range events {
		sessionID := agentSightSessionIdentity(event)
		if sessionFilter != "" && sessionID != sessionFilter {
			continue
		}
		summary.EventCount++
		var session *AgentSightSessionUsage
		if sessionID == "" {
			summary.UnattributedEvents++
		} else {
			session = sessions[sessionID]
			if session == nil {
				session = &AgentSightSessionUsage{ID: sessionID}
				sessions[sessionID] = session
			}
			session.EventCount++
			if event.Timestamp > session.LastEventTimestamp {
				session.LastEventTimestamp = event.Timestamp
			}
		}
		usage, model, ok := agentSightExplicitUsage(event)
		if !ok {
			continue
		}
		// Several events can describe the same completed model call (e.g.
		// HTTP response and OTLP span). Deduplicate only with a known call ID.
		if callID := agentSightCallIdentity(event); callID != "" {
			key := sessionID + "|" + callID
			if _, exists := seenCalls[key]; exists {
				continue
			}
			seenCalls[key] = struct{}{}
		}
		summary.UsageEvents++
		summary.InputTokens += usage.Input
		summary.OutputTokens += usage.Output
		summary.CacheReadTokens += usage.CacheRead
		summary.CacheCreationTokens += usage.CacheCreation
		if session != nil {
			session.UsageEvents++
			session.InputTokens += usage.Input
			session.OutputTokens += usage.Output
			session.CacheReadTokens += usage.CacheRead
			session.CacheCreationTokens += usage.CacheCreation
		}
		if model == "" {
			model = "unknown"
		}
		item := models[model]
		if item == nil {
			item = &AgentSightModelUsage{Model: model}
			models[model] = item
		}
		item.UsageEvents++
		item.InputTokens += usage.Input
		item.OutputTokens += usage.Output
		item.CacheReadTokens += usage.CacheRead
		item.CacheCreationTokens += usage.CacheCreation
	}
	for _, session := range sessions {
		summary.Sessions = append(summary.Sessions, *session)
	}
	sort.Slice(summary.Sessions, func(i, j int) bool {
		if summary.Sessions[i].LastEventTimestamp == summary.Sessions[j].LastEventTimestamp {
			return summary.Sessions[i].ID < summary.Sessions[j].ID
		}
		return summary.Sessions[i].LastEventTimestamp > summary.Sessions[j].LastEventTimestamp
	})
	for _, model := range models {
		summary.Models = append(summary.Models, *model)
	}
	sort.Slice(summary.Models, func(i, j int) bool {
		if summary.Models[i].InputTokens == summary.Models[j].InputTokens {
			return summary.Models[i].Model < summary.Models[j].Model
		}
		return summary.Models[i].InputTokens > summary.Models[j].InputTokens
	})
	return summary
}

func agentSightSessionIdentity(event AgentSightExportEvent) string {
	data := event.Data
	envelope := mapFromAny(data["envelope"])
	for _, m := range []map[string]any{data, envelope} {
		if id := firstString(m, "agent_run_id", "agentRunId", "session_id", "sessionId"); id != "" {
			return id
		}
	}
	if event.TraceID != "" {
		return "trace:" + event.TraceID
	}
	if id := firstString(data, "trace_id", "traceId"); id != "" {
		return "trace:" + id
	}
	return ""
}

func agentSightCallIdentity(event AgentSightExportEvent) string {
	if event.SpanID != "" {
		return "span:" + event.SpanID
	}
	if id := firstString(event.Data, "response_id", "responseId", "llm_call_id", "llmCallId"); id != "" {
		return "call:" + id
	}
	return ""
}

func agentSightExplicitUsage(event AgentSightExportEvent) (agentSightTokenUsage, string, bool) {
	data := event.Data
	attributes := mapFromAny(data["attributes"])
	envelope := mapFromAny(data["envelope"])
	candidates := []map[string]any{
		mapFromAny(data["usage"]), mapFromAny(data["token_usage"]),
		mapFromAny(data["tokenUsage"]), mapFromAny(data["llm_call"]),
		mapFromAny(data["llmCall"]), mapFromAny(envelope["llm_call"]),
		mapFromAny(envelope["llmCall"]), attributes, data,
	}
	model := firstString(data, "model", "model_name", "modelName", "gen_ai.request.model", "gen_ai.response.model")
	if model == "" {
		model = firstString(attributes, "gen_ai.response.model", "gen_ai.request.model", "model")
	}
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		input, hasInput := firstTokenNumber(candidate, "input_tokens", "inputTokens", "prompt_tokens", "promptTokens", "gen_ai.usage.input_tokens")
		output, hasOutput := firstTokenNumber(candidate, "output_tokens", "outputTokens", "completion_tokens", "completionTokens", "gen_ai.usage.output_tokens")
		cacheRead, hasCacheRead := firstTokenNumber(candidate, "cache_read_input_tokens", "cacheReadInputTokens", "cached_tokens", "cachedTokens")
		cacheCreation, hasCacheCreation := firstTokenNumber(candidate, "cache_creation_input_tokens", "cacheCreationInputTokens")
		if !hasInput && !hasOutput && !hasCacheRead && !hasCacheCreation {
			continue
		}
		if model == "" {
			model = firstString(candidate, "model", "model_name", "modelName")
		}
		return agentSightTokenUsage{input, output, cacheRead, cacheCreation}, model, true
	}
	return agentSightTokenUsage{}, model, false
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstTokenNumber(m map[string]any, keys ...string) (uint64, bool) {
	for _, key := range keys {
		value, exists := m[key]
		if !exists || value == nil {
			continue
		}
		switch n := value.(type) {
		case uint64:
			return n, true
		case int:
			if n >= 0 {
				return uint64(n), true
			}
		case int64:
			if n >= 0 {
				return uint64(n), true
			}
		case float64:
			if !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= 1e15 && math.Trunc(n) == n {
				return uint64(n), true
			}
		case json.Number:
			if v, err := strconv.ParseUint(string(n), 10, 64); err == nil {
				return v, true
			}
		case string:
			if v, err := strconv.ParseUint(strings.TrimSpace(n), 10, 64); err == nil {
				return v, true
			}
		}
	}
	return 0, false
}
