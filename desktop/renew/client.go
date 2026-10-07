package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type apiClient struct {
	base  *url.URL
	token string
	http  *http.Client
	dial  websocket.Dialer
}

type eventSummary struct {
	RootAgentPID   uint32  `json:"rootAgentPid,omitempty"`
	Key            string  `json:"key"`
	EventID        string  `json:"eventId"`
	PID            uint32  `json:"pid"`
	PPID           uint32  `json:"ppid"`
	UID            uint32  `json:"uid"`
	Type           string  `json:"type"`
	EventType      int32   `json:"eventType"`
	Tag            string  `json:"tag"`
	Comm           string  `json:"comm"`
	Path           string  `json:"path"`
	ExtraPath      string  `json:"extraPath,omitempty"`
	NetDirection   string  `json:"netDirection,omitempty"`
	NetEndpoint    string  `json:"netEndpoint,omitempty"`
	NetBytes       uint64  `json:"netBytes,omitempty"`
	Domain         string  `json:"domain,omitempty"`
	Decision       string  `json:"decision,omitempty"`
	RiskScore      float64 `json:"riskScore,omitempty"`
	AgentRunID     string  `json:"agentRunId,omitempty"`
	ConversationID string  `json:"conversationId,omitempty"`
	TurnID         string  `json:"turnId,omitempty"`
	ToolCallID     string  `json:"toolCallId,omitempty"`
	ToolName       string  `json:"toolName,omitempty"`
	TraceID        string  `json:"traceId,omitempty"`
	SpanID         string  `json:"spanId,omitempty"`
	Time           string  `json:"time"`
	ReceivedAtMS   int64   `json:"receivedAtMs"`
}

type summaryResponse struct {
	Events     []eventSummary `json:"events"`
	NextCursor string         `json:"nextCursor"`
	HasMore    bool           `json:"hasMore"`
}

type summaryBatch struct {
	Events []eventSummary `json:"events"`
}

type runtimeConfigResponse struct {
	Runtime                map[string]any `json:"runtime"`
	PersistedEventLogPath  string         `json:"persistedEventLogPath"`
	PersistedEventLogAlive bool           `json:"persistedEventLogAlive"`
}

type wrapperRule struct {
	Comm         string   `json:"comm"`
	Action       string   `json:"action"`
	RewrittenCmd []string `json:"rewritten_cmd,omitempty"`
	Regex        string   `json:"regex,omitempty"`
	Replacement  string   `json:"replacement,omitempty"`
	Priority     int      `json:"priority,omitempty"`
}

func newAPIClient(origin, token string) (*apiClient, error) {
	base, err := url.Parse(strings.TrimRight(origin, "/"))
	if err != nil || base.Host == "" {
		return nil, fmt.Errorf("invalid backend origin %q", origin)
	}
	return &apiClient{
		base:  base,
		token: strings.TrimSpace(token),
		http:  &http.Client{Timeout: 8 * time.Second},
		dial:  websocket.Dialer{HandshakeTimeout: 8 * time.Second, Proxy: http.ProxyFromEnvironment},
	}, nil
}

func (c *apiClient) endpoint(path string, query url.Values) string {
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	if query != nil {
		u.RawQuery = query.Encode()
	} else {
		u.RawQuery = ""
	}
	u.Fragment = ""
	return u.String()
}

func (c *apiClient) authHeaders() http.Header {
	h := make(http.Header)
	if c.token != "" {
		h.Set("X-API-KEY", c.token)
		h.Set("Authorization", "Bearer "+c.token)
	}
	return h
}

func (c *apiClient) doJSON(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path, query), reader)
	if err != nil {
		return err
	}
	for key, values := range c.authHeaders() {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(string(payload))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("backend returned %s: %s", resp.Status, message)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *apiClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, query, nil, out)
}

func (c *apiClient) summaries(ctx context.Context, limit int) ([]eventSummary, error) {
	q := make(url.Values)
	q.Set("limit", strconv.Itoa(limit))
	var response summaryResponse
	if err := c.getJSON(ctx, "/events/summaries", q, &response); err != nil {
		return nil, err
	}
	return response.Events, nil
}

func (c *apiClient) eventDetail(ctx context.Context, eventID string) (string, error) {
	var detail json.RawMessage
	if err := c.getJSON(ctx, "/events/detail/"+eventID, nil, &detail); err != nil {
		return "", err
	}
	pretty, err := json.MarshalIndent(detail, "", "  ")
	if err != nil {
		return string(detail), nil
	}
	return string(pretty), nil
}

func (c *apiClient) runtimeConfig(ctx context.Context) (runtimeConfigResponse, error) {
	var response runtimeConfigResponse
	err := c.getJSON(ctx, "/config/runtime", nil, &response)
	return response, err
}

func (c *apiClient) patchRuntime(ctx context.Context, patch map[string]any) (runtimeConfigResponse, error) {
	var response runtimeConfigResponse
	err := c.doJSON(ctx, http.MethodPut, "/config/runtime", nil, patch, &response)
	return response, err
}

func (c *apiClient) rules(ctx context.Context) ([]wrapperRule, error) {
	var raw json.RawMessage
	if err := c.getJSON(ctx, "/config/rules", nil, &raw); err != nil {
		return nil, err
	}

	var list []wrapperRule
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var keyed map[string]wrapperRule
	if err := json.Unmarshal(raw, &keyed); err != nil {
		return nil, fmt.Errorf("decode wrapper rules: %w", err)
	}
	list = make([]wrapperRule, 0, len(keyed))
	for key, rule := range keyed {
		if strings.TrimSpace(rule.Comm) == "" {
			rule.Comm = key
		}
		list = append(list, rule)
	}
	return list, nil
}

func (c *apiClient) saveRule(ctx context.Context, rule wrapperRule) error {
	return c.doJSON(ctx, http.MethodPost, "/config/rules", nil, rule, nil)
}

func (c *apiClient) deleteRule(ctx context.Context, comm string) error {
	return c.doJSON(ctx, http.MethodDelete, "/config/rules/"+comm, nil, nil, nil)
}

func (c *apiClient) dialWebSocket(ctx context.Context, path string, query url.Values) (*websocket.Conn, *http.Response, error) {
	u := *c.base
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	if query != nil {
		u.RawQuery = query.Encode()
	} else {
		u.RawQuery = ""
	}
	u.Fragment = ""
	return c.dial.DialContext(ctx, u.String(), c.authHeaders())
}
