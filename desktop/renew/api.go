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
	EventID        string  `json:"eventId"`
	PID            int     `json:"pid"`
	PPID           int     `json:"ppid"`
	RootAgentPID   int     `json:"rootAgentPid"`
	Type           string  `json:"type"`
	Tag            string  `json:"tag"`
	Comm           string  `json:"comm"`
	Path           string  `json:"path"`
	ExtraPath      string  `json:"extraPath"`
	NetDirection   string  `json:"netDirection"`
	NetEndpoint    string  `json:"netEndpoint"`
	NetBytes       int64   `json:"netBytes"`
	Domain         string  `json:"domain"`
	Decision       string  `json:"decision"`
	RiskScore      float64 `json:"riskScore"`
	AgentRunID     string  `json:"agentRunId"`
	ConversationID string  `json:"conversationId"`
	ToolCallID     string  `json:"toolCallId"`
	ToolName       string  `json:"toolName"`
	ReceivedAtMS   int64   `json:"receivedAtMs"`
}

type eventSummaryResponse struct {
	Events     []eventSummary `json:"events"`
	NextCursor string         `json:"nextCursor"`
}

type collectorHealth struct {
	CaptureHealthy      bool  `json:"captureHealthy"`
	RingbufDroppedTotal int64 `json:"ringbufDroppedTotal"`
}

type eventTypeConfig struct {
	DisabledEventTypes []int `json:"disabled_event_types"`
}

type wrapperRule struct {
	Comm         string   `json:"comm"`
	Action       string   `json:"action"`
	RewrittenCmd []string `json:"rewritten_cmd,omitempty"`
	Regex        string   `json:"regex,omitempty"`
	Replacement  string   `json:"replacement,omitempty"`
	Priority     int      `json:"priority,omitempty"`
}

type apiSnapshot struct {
	Events       []eventSummary
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
		http:   &http.Client{Timeout: 3 * time.Second},
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

func (c *apiClient) snapshot(ctx context.Context, limit int) (apiSnapshot, error) {
	if limit <= 0 {
		limit = 240
	}
	var events eventSummaryResponse
	if err := c.getJSON(ctx, "/events/summaries?limit="+strconv.Itoa(limit), &events); err != nil {
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
		Health:       health,
		TrackedComms: tracked,
		FetchedAt:    time.Now(),
	}, nil
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

func eventTarget(e eventSummary) string {
	for _, value := range []string{e.Path, e.NetEndpoint, e.Domain, e.ExtraPath, e.ToolName} {
		if strings.TrimSpace(value) != "" {
			return value
		}
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
	return strings.ToLower(strings.Join([]string{
		e.EventID, e.Type, e.Tag, e.Comm, e.Path, e.ExtraPath, e.NetEndpoint,
		e.Domain, e.Decision, e.AgentRunID, e.ConversationID, e.ToolName,
		url.QueryEscape(e.EventID),
	}, " "))
}
