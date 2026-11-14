// Package remote is the read-only, transport-independent client for Agent eBPF.
// It deliberately does not import MyGo, so the protocol can be tested on Linux.
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 2 << 20

type Event struct {
	EventID     string  `json:"eventId"`
	PID         int     `json:"pid"`
	Type        string  `json:"type"`
	Tag         string  `json:"tag"`
	Comm        string  `json:"comm"`
	Target      string  `json:"target"`
	Domain      string  `json:"domain"`
	NetEndpoint string  `json:"netEndpoint"`
	Decision    string  `json:"decision"`
	RiskScore   float64 `json:"riskScore"`
	ReceivedAt  int64   `json:"receivedAtMs"`
	AgentRunID  string  `json:"agentRunId"`
}

type Health struct {
	CaptureHealthy      bool  `json:"captureHealthy"`
	RingbufDroppedTotal int64 `json:"ringbufDroppedTotal"`
	BackendQueueLen     int64 `json:"backendQueueLen"`
	PersistQueueLen     int64 `json:"persistQueueLen"`
	PersistQueueCap     int64 `json:"persistQueueCap"`
	PersistPending      int64 `json:"persistPending"`
}

type EventPage struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"nextCursor"`
}

type Snapshot struct {
	Health    Health
	Events    []Event
	CheckedAt time.Time
}

type Client struct {
	base  string
	token string
	http  *http.Client
}

// NormalizeOrigin accepts a backend origin, not an arbitrary URL. Remote
// connections must use TLS to prevent API tokens and process paths being
// exposed on Wi-Fi. The only HTTP exemption is local loopback for debugging.
func NormalizeOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return "", errors.New("invalid backend address")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("use an https:// backend address")
	}
	if u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Path != "" || u.RawPath != "" || u.Opaque != "" {
		return "", errors.New("enter only a backend origin, e.g. https://monitor.example.com")
	}
	if strings.ContainsAny(u.Hostname(), " \r\n\t") {
		return "", errors.New("invalid hostname")
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", errors.New("remote HTTP is disabled; configure HTTPS or a trusted TLS proxy")
		}
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", errors.New("invalid backend port")
		}
	}
	return u.Scheme + "://" + u.Host, nil
}

func NewClient(origin, token string) (*Client, error) {
	base, err := NormalizeOrigin(origin)
	if err != nil {
		return nil, err
	}
	return &Client{
		base: base,
		token: strings.TrimSpace(token),
		http: &http.Client{
			Timeout: 6 * time.Second,
			// Never follow a redirect with a credential-bearing request.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("X-API-KEY", c.token)
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("backend unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("access denied: check your API token")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("backend returned HTTP %d", resp.StatusCode)
	}
	reader := io.LimitReader(resp.Body, maxResponseBytes+1)
	payload, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read backend response: %w", err)
	}
	if len(payload) > maxResponseBytes {
		return errors.New("backend response exceeds mobile size limit")
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("invalid backend JSON: %w", err)
	}
	return nil
}

// FetchSnapshot uses exactly two GET endpoints and never changes server policy.
func (c *Client) FetchSnapshot(ctx context.Context, limit int) (Snapshot, error) {
	if limit < 1 || limit > 100 {
		limit = 60
	}
	var health Health
	if err := c.get(ctx, "/system/collector-health", &health); err != nil {
		return Snapshot{}, err
	}
	var page EventPage
	if err := c.get(ctx, "/events/summaries?compact=1&limit="+strconv.Itoa(limit), &page); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Health: health, Events: page.Events, CheckedAt: time.Now()}, nil
}

// FetchEventDetail loads one event only when the user explicitly opens it.
func (c *Client) FetchEventDetail(ctx context.Context, eventID string) (map[string]any, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" || len(eventID) > 256 || strings.ContainsAny(eventID, "/\\?#") {
		return nil, errors.New("invalid event identifier")
	}
	var detail map[string]any
	if err := c.get(ctx, "/events/detail/"+url.PathEscape(eventID), &detail); err != nil {
		return nil, err
	}
	return detail, nil
}
