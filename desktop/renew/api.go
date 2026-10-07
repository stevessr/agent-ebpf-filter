package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type eventSummary struct {
	EventID          string  `json:"eventId"`
	PID              int     `json:"pid"`
	PPID             int     `json:"ppid"`
	RootAgentPID     int     `json:"rootAgentPid"`
	EventType        int     `json:"eventType"`
	Type             string  `json:"type"`
	Tag              string  `json:"tag"`
	Comm             string  `json:"comm"`
	Target           string  `json:"target"`
	Network          bool    `json:"network"`
	NetBytes         int64   `json:"netBytes"`
	Decision         string  `json:"decision"`
	RiskScore        float64 `json:"riskScore"`
	AgentRunID       string  `json:"agentRunId"`
	ConversationID   string  `json:"conversationId"`
	ToolName         string  `json:"toolName"`
	HasAgentContext  bool    `json:"hasAgentContext"`
	ReceivedAtMS     int64   `json:"receivedAtMs"`
	SearchText       string  `json:"-"`
}

type eventSummaryResponse struct {
	Events     []eventSummary `json:"events"`
	NextCursor string         `json:"nextCursor"`
}

type collectorHealth struct {
	CaptureHealthy      bool  `json:"captureHealthy"`
	RingbufDroppedTotal int64 `json:"ringbufDroppedTotal"`
	BackendQueueLen     int64 `json:"backendQueueLen"`
	PersistQueueLen     int64 `json:"persistQueueLen"`
	PersistQueueCap     int64 `json:"persistQueueCap"`
	PersistPending      int64 `json:"persistPending"`
}

type eventTypeConfig struct {
	DisabledEventTypes []int `json:"disabled_event_types"`
}

type runtimeToggle struct {
	Enabled bool `json:"enabled"`
}

type runtimeSettings struct {
	LogPersistenceEnabled   bool          `json:"logPersistenceEnabled"`
	LogFilePath             string        `json:"logFilePath"`
	DisabledEventTypes      []int         `json:"disabledEventTypes"`
	PolicyManagementEnabled bool          `json:"policyManagementEnabled"`
	TLSCaptureEnabled       bool          `json:"tlsCaptureEnabled"`
	LoopDetection           runtimeToggle `json:"loopDetection"`
	SignalProcessing        runtimeToggle `json:"signalProcessing"`
	ResearchProcessing      runtimeToggle `json:"researchProcessing"`
}

type runtimeConfigResponse struct {
	Runtime                runtimeSettings `json:"runtime"`
	PersistedEventLogPath  string          `json:"persistedEventLogPath"`
	PersistedEventLogAlive bool            `json:"persistedEventLogAlive"`
}

type wrapperRule struct {
	Comm         string   `json:"comm"`
	Action       string   `json:"action"`
	RewrittenCmd []string `json:"rewritten_cmd,omitempty"`
	Regex        string   `json:"regex,omitempty"`
	Replacement  string   `json:"replacement,omitempty"`
	Priority     int      `json:"priority,omitempty"`
}

type trackedComm struct {
	Comm     string `json:"comm"`
	Tag      string `json:"tag"`
	Disabled bool   `json:"disabled"`
}

type trackedPath struct {
	Path string `json:"path"`
	Tag  string `json:"tag"`
}

type trackedPrefix struct {
	Prefix string `json:"prefix"`
	Tag    string `json:"tag"`
}

type registrySnapshot struct {
	Tags     []string
	Comms    []trackedComm
	Paths    []trackedPath
	Prefixes []trackedPrefix
}

type cgroupSandboxStatus struct {
	Available    bool     `json:"available"`
	Attached     bool     `json:"attached"`
	BlockedIPs   []string `json:"blockedIPs"`
	BlockedPorts []int    `json:"blockedPorts"`
	Error        string   `json:"error"`
}

type lsmSandboxStatus struct {
	Available        bool     `json:"available"`
	Attached         bool     `json:"attached"`
	BlockedExecPaths []string `json:"blockedExecPaths"`
	Error            string   `json:"error"`
}

type enforcementSnapshot struct {
	Cgroup cgroupSandboxStatus
	LSM    lsmSandboxStatus
}

type apiSnapshot struct {
	Events       []eventSummary
	NextCursor   string
	Health       collectorHealth
	TrackedComms []string
	FetchedAt    time.Time
}

type apiClient struct {
	origin string
	token  string
	http   *http.Client
}

func newAPIClient(origin, token string) *apiClient {
	return &apiClient{
		origin: strings.TrimRight(origin, "/"),
		token:  strings.TrimSpace(token),
		http:   &http.Client{Timeout: 4 * time.Second},
	}
}

func (c *apiClient) requestJSON(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s: encode request: %w", path, err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("X-API-KEY", c.token)
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
		var response struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &response) == nil && strings.TrimSpace(response.Error) != "" {
			return fmt.Errorf("%s: %s", path, response.Error)
		}
		return fmt.Errorf("%s: backend returned %s", path, res.Status)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func (c *apiClient) getJSON(ctx context.Context, path string, out any) error {
	return c.requestJSON(ctx, http.MethodGet, path, nil, out)
}

func (c *apiClient) eventSummaries(ctx context.Context, limit int, cursor string) (eventSummaryResponse, error) {
	if limit <= 0 {
		limit = 240
	}
	values := url.Values{}
	values.Set("limit", strconv.Itoa(limit))
	values.Set("compact", "1")
	if strings.TrimSpace(cursor) != "" {
		values.Set("cursor", cursor)
	}
	var response eventSummaryResponse
	err := c.getJSON(ctx, "/events/summaries?"+values.Encode(), &response)
	return response, err
}

func (c *apiClient) snapshot(ctx context.Context, limit int) (apiSnapshot, error) {
	events, err := c.eventSummaries(ctx, limit, "")
	if err != nil {
		return apiSnapshot{}, err
	}
	var health collectorHealth
	if err := c.getJSON(ctx, "/system/collector-health", &health); err != nil {
		return apiSnapshot{}, err
	}
	var tracked []string
	if err := c.getJSON(ctx, "/system/tracked-comms", &tracked); err != nil {
		tracked = nil
	}
	return apiSnapshot{
		Events:       events.Events,
		NextCursor:   events.NextCursor,
		Health:       health,
		TrackedComms: tracked,
		FetchedAt:    time.Now(),
	}, nil
}

func (c *apiClient) statusSnapshot(ctx context.Context) (apiSnapshot, error) {
	var health collectorHealth
	if err := c.getJSON(ctx, "/system/collector-health", &health); err != nil {
		return apiSnapshot{}, err
	}
	var tracked []string
	if err := c.getJSON(ctx, "/system/tracked-comms", &tracked); err != nil {
		tracked = nil
	}
	return apiSnapshot{
		Health:       health,
		TrackedComms: tracked,
		FetchedAt:    time.Now(),
	}, nil
}

func (c *apiClient) eventDetail(ctx context.Context, eventID string) (map[string]any, error) {
	var detail map[string]any
	err := c.getJSON(ctx, "/events/detail/"+url.PathEscape(strings.TrimSpace(eventID)), &detail)
	return detail, err
}

func (c *apiClient) eventTypeConfig(ctx context.Context) (eventTypeConfig, error) {
	var cfg eventTypeConfig
	err := c.getJSON(ctx, "/config/event-types", &cfg)
	return cfg, err
}

func (c *apiClient) putEventTypeConfig(ctx context.Context, disabled []int) (eventTypeConfig, error) {
	var cfg eventTypeConfig
	err := c.requestJSON(ctx, http.MethodPut, "/config/event-types", eventTypeConfig{DisabledEventTypes: disabled}, &cfg)
	return cfg, err
}

func (c *apiClient) runtimeConfig(ctx context.Context) (runtimeConfigResponse, error) {
	var cfg runtimeConfigResponse
	err := c.getJSON(ctx, "/config/runtime", &cfg)
	return cfg, err
}

func (c *apiClient) putRuntimeConfig(ctx context.Context, patch map[string]any) (runtimeConfigResponse, error) {
	var cfg runtimeConfigResponse
	err := c.requestJSON(ctx, http.MethodPut, "/config/runtime", patch, &cfg)
	return cfg, err
}

func (c *apiClient) runtimePatchWithNestedToggle(ctx context.Context, key string, enabled bool) (runtimeConfigResponse, error) {
	var current map[string]any
	if err := c.getJSON(ctx, "/config/runtime", &current); err != nil {
		return runtimeConfigResponse{}, err
	}
	runtimeMap, _ := current["runtime"].(map[string]any)
	if runtimeMap == nil {
		return runtimeConfigResponse{}, fmt.Errorf("/config/runtime: missing runtime payload")
	}

	patch := map[string]any{}
	switch key {
	case "loopDetection", "signalProcessing", "researchProcessing":
		nested, _ := runtimeMap[key].(map[string]any)
		if nested == nil {
			nested = map[string]any{}
		}
		next := make(map[string]any, len(nested)+1)
		for field, value := range nested {
			next[field] = value
		}
		next["enabled"] = enabled
		patch[key] = next
	case "tlsCapture":
		patch["tlsCaptureEnabled"] = enabled
	case "persistence":
		patch["logPersistenceEnabled"] = enabled
	case "policyManagement":
		patch["policyManagementEnabled"] = enabled
	default:
		return runtimeConfigResponse{}, fmt.Errorf("unsupported runtime toggle %q", key)
	}
	return c.putRuntimeConfig(ctx, patch)
}

func (c *apiClient) applyMonitoringProfile(ctx context.Context, disabled []int, loop, signal, research bool) (runtimeConfigResponse, error) {
	var current map[string]any
	if err := c.getJSON(ctx, "/config/runtime", &current); err != nil {
		return runtimeConfigResponse{}, err
	}
	runtimeMap, _ := current["runtime"].(map[string]any)
	if runtimeMap == nil {
		return runtimeConfigResponse{}, fmt.Errorf("/config/runtime: missing runtime payload")
	}
	patch := map[string]any{"disabledEventTypes": disabled}
	for key, enabled := range map[string]bool{
		"loopDetection":      loop,
		"signalProcessing":   signal,
		"researchProcessing": research,
	} {
		nested, _ := runtimeMap[key].(map[string]any)
		if nested == nil {
			nested = map[string]any{}
		}
		next := make(map[string]any, len(nested)+1)
		for field, value := range nested {
			next[field] = value
		}
		next["enabled"] = enabled
		patch[key] = next
	}
	return c.putRuntimeConfig(ctx, patch)
}

func (c *apiClient) rules(ctx context.Context) ([]wrapperRule, error) {
	var raw json.RawMessage
	if err := c.getJSON(ctx, "/config/rules", &raw); err != nil {
		return nil, err
	}
	var rules []wrapperRule
	if err := json.Unmarshal(raw, &rules); err != nil {
		var keyed map[string]wrapperRule
		if objectErr := json.Unmarshal(raw, &keyed); objectErr != nil {
			return nil, fmt.Errorf("/config/rules: decode: %w", err)
		}
		rules = make([]wrapperRule, 0, len(keyed))
		for key, rule := range keyed {
			if rule.Comm == "" {
				rule.Comm = key
			}
			rules = append(rules, rule)
		}
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		return rules[i].Comm < rules[j].Comm
	})
	return rules, nil
}

func (c *apiClient) saveRule(ctx context.Context, rule wrapperRule) error {
	return c.requestJSON(ctx, http.MethodPost, "/config/rules", rule, nil)
}

func (c *apiClient) deleteRule(ctx context.Context, comm string) error {
	return c.requestJSON(ctx, http.MethodDelete, "/config/rules/"+url.PathEscape(comm), nil, nil)
}

func (c *apiClient) registry(ctx context.Context) (registrySnapshot, error) {
	var out registrySnapshot
	if err := c.getJSON(ctx, "/config/tags", &out.Tags); err != nil {
		return out, err
	}
	if err := c.getJSON(ctx, "/config/comms", &out.Comms); err != nil {
		return out, err
	}
	if err := c.getJSON(ctx, "/config/paths", &out.Paths); err != nil {
		return out, err
	}
	if err := c.getJSON(ctx, "/config/prefixes", &out.Prefixes); err != nil {
		return out, err
	}
	sort.Strings(out.Tags)
	sort.Slice(out.Comms, func(i, j int) bool { return out.Comms[i].Comm < out.Comms[j].Comm })
	sort.Slice(out.Paths, func(i, j int) bool { return out.Paths[i].Path < out.Paths[j].Path })
	sort.Slice(out.Prefixes, func(i, j int) bool { return out.Prefixes[i].Prefix < out.Prefixes[j].Prefix })
	return out, nil
}

func (c *apiClient) addTag(ctx context.Context, name string) error {
	return c.requestJSON(ctx, http.MethodPost, "/config/tags", map[string]any{"name": strings.TrimSpace(name)}, nil)
}

func (c *apiClient) addComm(ctx context.Context, comm, tag string) error {
	return c.requestJSON(ctx, http.MethodPost, "/config/comms", map[string]any{"comm": strings.TrimSpace(comm), "tag": strings.TrimSpace(tag)}, nil)
}

func (c *apiClient) deleteComm(ctx context.Context, comm string) error {
	return c.requestJSON(ctx, http.MethodDelete, "/config/comms/"+url.PathEscape(strings.TrimSpace(comm)), nil, nil)
}

func (c *apiClient) setCommDisabled(ctx context.Context, comm string, disabled bool) error {
	method := http.MethodPost
	if !disabled {
		method = http.MethodDelete
	}
	return c.requestJSON(ctx, method, "/config/comms/"+url.PathEscape(strings.TrimSpace(comm))+"/disable", nil, nil)
}

func (c *apiClient) addPath(ctx context.Context, path, tag string) error {
	return c.requestJSON(ctx, http.MethodPost, "/config/paths", map[string]any{"path": strings.TrimSpace(path), "tag": strings.TrimSpace(tag)}, nil)
}

func (c *apiClient) deletePath(ctx context.Context, path string) error {
	// The backend wildcard route intentionally receives an extra slash for
	// absolute paths (the browser Renew client uses the same shape).
	return c.requestJSON(ctx, http.MethodDelete, "/config/paths/"+strings.TrimSpace(path), nil, nil)
}

func (c *apiClient) addPrefix(ctx context.Context, prefix, tag string) error {
	return c.requestJSON(ctx, http.MethodPost, "/config/prefixes", map[string]any{"prefix": strings.TrimSpace(prefix), "tag": strings.TrimSpace(tag)}, nil)
}

func (c *apiClient) deletePrefix(ctx context.Context, prefix string) error {
	values := url.Values{}
	values.Set("prefix", strings.TrimSpace(prefix))
	return c.requestJSON(ctx, http.MethodDelete, "/config/prefixes?"+values.Encode(), nil, nil)
}

func (c *apiClient) enforcementStatus(ctx context.Context) (enforcementSnapshot, error) {
	var out enforcementSnapshot
	cgroupErr := c.getJSON(ctx, "/sandbox/cgroup/status", &out.Cgroup)
	lsmErr := c.getJSON(ctx, "/sandbox/lsm/status", &out.LSM)
	if cgroupErr != nil && lsmErr != nil {
		return out, fmt.Errorf("sandbox status: cgroup: %v; lsm: %v", cgroupErr, lsmErr)
	}
	return out, nil
}

func (c *apiClient) enforcementAction(ctx context.Context, path string, payload map[string]any) error {
	return c.requestJSON(ctx, http.MethodPost, path, payload, nil)
}

func eventTarget(e eventSummary) string {
	if strings.TrimSpace(e.Target) != "" {
		return e.Target
	}
	if strings.TrimSpace(e.ToolName) != "" {
		return e.ToolName
	}
	return "-"
}

func eventAction(e eventSummary) string {
	if e.ToolName != "" {
		return "调用工具 " + e.ToolName
	}
	t := strings.ToLower(e.Type)
	switch {
	case strings.Contains(t, "network"), strings.Contains(t, "connect"), strings.Contains(t, "socket"), strings.Contains(t, "tcp"), strings.Contains(t, "dns"):
		return "访问网络"
	case strings.Contains(t, "write"), strings.Contains(t, "rename"), strings.Contains(t, "unlink"):
		return "修改文件"
	case strings.Contains(t, "open"), strings.Contains(t, "read"), strings.Contains(t, "file"):
		return "读取文件"
	case strings.Contains(t, "exec"), strings.Contains(t, "process"), strings.Contains(t, "clone"):
		return "进程活动"
	default:
		if e.Type != "" {
			return e.Type
		}
		return "系统活动"
	}
}

func eventTime(e eventSummary) string {
	if e.ReceivedAtMS <= 0 {
		return "--:--:--"
	}
	return time.UnixMilli(e.ReceivedAtMS).Format("15:04:05")
}

func eventRisk(e eventSummary) string {
	decision := strings.ToUpper(strings.TrimSpace(e.Decision))
	if decision == "BLOCK" || decision == "DENY" || e.RiskScore >= 80 {
		return "高风险"
	}
	if decision == "ALERT" || e.RiskScore >= 60 {
		return "需关注"
	}
	return "正常"
}

func eventSearchText(e eventSummary) string {
	if e.SearchText != "" {
		return e.SearchText
	}
	return buildEventSearchText(e)
}

func buildEventSearchText(e eventSummary) string {
	return strings.ToLower(strings.Join([]string{
		e.EventID, e.Type, e.Tag, e.Comm, e.Target, e.Decision,
		e.AgentRunID, e.ConversationID, e.ToolName, strconv.Itoa(e.PID),
		url.QueryEscape(e.EventID),
	}, " "))
}
