package main

import (
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
	u.RawQuery = query.Encode()
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

func (c *apiClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(path, query), nil)
	if err != nil {
		return err
	}
	for key, values := range c.authHeaders() {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("backend returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
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
	if err := c.getJSON(ctx, "/events/detail/"+url.PathEscape(eventID), nil, &detail); err != nil {
		return "", err
	}
	pretty, err := json.MarshalIndent(detail, "", "  ")
	if err != nil {
		return string(detail), nil
	}
	return string(pretty), nil
}

func (c *apiClient) dialWebSocket(ctx context.Context, path string, query url.Values) (*websocket.Conn, *http.Response, error) {
	u := *c.base
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	u.RawQuery = query.Encode()
	u.Fragment = ""
	return c.dial.DialContext(ctx, u.String(), c.authHeaders())
}
