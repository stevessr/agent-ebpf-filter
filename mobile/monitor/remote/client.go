// Package remote is the read-only, transport-independent client for Agent eBPF.
// It deliberately does not import MyGo, so the protocol can be tested on Linux.
package remote

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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

const (
	maxResponseBytes  = 2 << 20
	maxRetainedEvents = 240
)

// ErrAccessDenied is returned when the backend rejects the API token (HTTP 401/403).
var ErrAccessDenied = errors.New("access denied: check your API token")

type Event struct {
	EventID         string  `json:"eventId"`
	PID             int     `json:"pid"`
	PPID            int     `json:"ppid"`
	RootAgentPID    int     `json:"rootAgentPid"`
	Type            string  `json:"type"`
	Tag             string  `json:"tag"`
	Comm            string  `json:"comm"`
	Target          string  `json:"target"`
	Domain          string  `json:"domain"`
	NetEndpoint     string  `json:"netEndpoint"`
	Network         bool    `json:"network"`
	NetBytes        uint64  `json:"netBytes"`
	Decision        string  `json:"decision"`
	RiskScore       float64 `json:"riskScore"`
	ReceivedAt      int64   `json:"receivedAtMs"`
	AgentRunID      string  `json:"agentRunId"`
	ConversationID  string  `json:"conversationId"`
	ToolName        string  `json:"toolName"`
	HasAgentContext bool    `json:"hasAgentContext"`
}

type Health struct {
	CaptureHealthy         bool  `json:"captureHealthy"`
	CollectorMapAvailable  bool  `json:"collectorMapAvailable"`
	RingbufEventsTotal     int64 `json:"ringbufEventsTotal"`
	RingbufDroppedTotal    int64 `json:"ringbufDroppedTotal"`
	BackendQueueLen        int64 `json:"backendQueueLen"`
	WsClients              int64 `json:"wsClients"`
	PersistWriterActive    bool  `json:"persistWriterActive"`
	PersistQueueLen        int64 `json:"persistQueueLen"`
	PersistQueueCap        int64 `json:"persistQueueCap"`
	PersistPending         int64 `json:"persistPending"`
	CapturedPersistedTotal int64 `json:"capturedPersistedTotal"`
	KernelRiskAlertsTotal  int64 `json:"kernelRiskAlertsTotal"`
	KernelRiskBlocksTotal  int64 `json:"kernelRiskBlocksTotal"`
}

type EventPage struct {
	Source     string  `json:"source"`
	Events     []Event `json:"events"`
	NextCursor string  `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

type Snapshot struct {
	Health     Health
	Events     []Event
	NextCursor string
	HasMore    bool
	CheckedAt  time.Time
	Latency    time.Duration
}

type Client struct {
	base  string
	token string
	http  *http.Client
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// IsLoopbackOrigin reports whether an origin targets local loopback (e.g. via USB adb reverse).
func IsLoopbackOrigin(origin string) bool {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u == nil {
		return false
	}
	return isLoopbackHost(u.Hostname())
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
	if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		return "", errors.New("remote HTTP is disabled; configure HTTPS or a trusted TLS proxy")
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", errors.New("invalid backend port")
		}
	}
	return u.Scheme + "://" + u.Host, nil
}

// CleanOriginInput normalizes mobile user input (such as a pasted origin with a
// trailing slash, or a bare host:port without an explicit scheme) before calling
// NormalizeOrigin. It never weakens NormalizeOrigin's security rules.
func CleanOriginInput(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || s == "https://" || s == "http://" {
		return s
	}
	if !strings.Contains(s, "://") {
		hostPart := s
		if idx := strings.IndexByte(hostPart, '/'); idx >= 0 {
			hostPart = hostPart[:idx]
		}
		probe, err := url.Parse("http://" + hostPart)
		if err == nil && probe != nil && isLoopbackHost(probe.Hostname()) {
			s = "http://" + s
		} else {
			s = "https://" + s
		}
	}
	if u, err := url.Parse(s); err == nil && u != nil && u.Path == "/" && u.RawQuery == "" && u.Fragment == "" {
		s = strings.TrimSuffix(s, "/")
	}
	return s
}

// ParseConnectInput parses either a direct origin + token form input or a
// mobile pairing / deep-link URI (agentebpf://connect?origin=...&token=... or
// renewmonitor://connect?origin=...). The returned origin is always validated
// through NormalizeOrigin.
func ParseConnectInput(rawOrigin, rawToken string) (string, string, error) {
	originCandidate := strings.TrimSpace(rawOrigin)
	tokenCandidate := strings.TrimSpace(rawToken)

	if u, err := url.Parse(originCandidate); err == nil && u != nil {
		scheme := strings.ToLower(u.Scheme)
		if scheme == "agentebpf" || scheme == "renewmonitor" {
			if !strings.EqualFold(u.Host, "connect") && !strings.EqualFold(strings.Trim(u.Path, "/"), "connect") {
				return "", "", errors.New("unsupported mobile connection link")
			}
			q := u.Query()
			extractedOrigin := firstNonEmptyString(q.Get("origin"), q.Get("backend"), q.Get("url"))
			if extractedOrigin == "" {
				return "", "", errors.New("connection link is missing origin parameter")
			}
			originCandidate = extractedOrigin
			if tokenCandidate == "" {
				tokenCandidate = strings.TrimSpace(q.Get("token"))
			}
		}
	}

	cleaned := CleanOriginInput(originCandidate)
	normalized, err := NormalizeOrigin(cleaned)
	if err != nil {
		return "", "", err
	}
	return normalized, tokenCandidate, nil
}

// FormatConnectURI builds a validated deep-link URI for pre-filling the mobile
// connection screen. By default tokens are omitted unless includeToken is true.
func FormatConnectURI(origin, token string, includeToken bool) (string, error) {
	normalized, err := NormalizeOrigin(CleanOriginInput(origin))
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("origin", normalized)
	if includeToken && strings.TrimSpace(token) != "" {
		q.Set("token", strings.TrimSpace(token))
	}
	return "agentebpf://connect?" + q.Encode(), nil
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

func NewClient(origin, token string) (*Client, error) {
	base, err := NormalizeOrigin(origin)
	if err != nil {
		return nil, err
	}
	return &Client{
		base:  base,
		token: strings.TrimSpace(token),
		http: &http.Client{
			Timeout: 6 * time.Second,
			// Never follow a redirect with a credential-bearing request.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// Origin returns the normalized backend origin of the client.
func (c *Client) Origin() string {
	if c == nil {
		return ""
	}
	return c.base
}

// HasToken reports whether the client was configured with a non-empty API token.
func (c *Client) HasToken() bool {
	return c != nil && c.token != ""
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
		return ErrAccessDenied
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

// Probe verifies reachability and authentication against /system/collector-health
// and returns the measured round-trip latency.
func (c *Client) Probe(ctx context.Context) (Health, time.Duration, error) {
	start := time.Now()
	var health Health
	if err := c.get(ctx, "/system/collector-health", &health); err != nil {
		return Health{}, 0, err
	}
	return health, time.Since(start), nil
}

// FetchEventsPage fetches one page of compact event summaries, optionally using
// nextCursor to load older persisted events.
func (c *Client) FetchEventsPage(ctx context.Context, limit int, cursor string) (EventPage, error) {
	if limit < 1 || limit > 100 {
		limit = 60
	}
	cursor = strings.TrimSpace(cursor)
	if len(cursor) > 512 || strings.ContainsAny(cursor, " \r\n\t/\\?#") {
		return EventPage{}, errors.New("invalid event cursor")
	}
	path := "/events/summaries?compact=1&limit=" + strconv.Itoa(limit)
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	var page EventPage
	if err := c.get(ctx, path, &page); err != nil {
		return EventPage{}, err
	}
	return page, nil
}

// FetchSnapshot uses exactly two GET endpoints and never changes server policy.
func (c *Client) FetchSnapshot(ctx context.Context, limit int) (Snapshot, error) {
	start := time.Now()
	var health Health
	if err := c.get(ctx, "/system/collector-health", &health); err != nil {
		return Snapshot{}, err
	}
	page, err := c.FetchEventsPage(ctx, limit, "")
	if err != nil {
		return Snapshot{}, err
	}
	now := time.Now()
	return Snapshot{
		Health:     health,
		Events:     page.Events,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore || page.NextCursor != "",
		CheckedAt:  now,
		Latency:    now.Sub(start),
	}, nil
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

// MergeEventPages appends older events to an existing slice while deduplicating
// by EventID and capping retained events for bounded mobile memory usage.
func MergeEventPages(existing, older []Event, maxEvents int) []Event {
	if maxEvents <= 0 {
		maxEvents = maxRetainedEvents
	}
	seen := make(map[string]struct{}, len(existing)+len(older))
	out := make([]Event, 0, min(len(existing)+len(older), maxEvents))
	for _, ev := range existing {
		if len(out) >= maxEvents {
			return out
		}
		if ev.EventID != "" {
			if _, ok := seen[ev.EventID]; ok {
				continue
			}
			seen[ev.EventID] = struct{}{}
		}
		out = append(out, ev)
	}
	for _, ev := range older {
		if len(out) >= maxEvents {
			break
		}
		if ev.EventID != "" {
			if _, ok := seen[ev.EventID]; ok {
				continue
			}
			seen[ev.EventID] = struct{}{}
		}
		out = append(out, ev)
	}
	return out
}

// IsAuthError reports whether err represents an authentication/authorization failure.
func IsAuthError(err error) bool {
	return errors.Is(err, ErrAccessDenied)
}

// IsTLSError reports whether err is a TLS handshake or certificate validation failure.
func IsTLSError(err error) bool {
	if err == nil {
		return false
	}
	var certInvalid x509.CertificateInvalidError
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var sysRoots x509.SystemRootsError
	var recordHeader tls.RecordHeaderError
	if errors.As(err, &certInvalid) || errors.As(err, &unknownAuth) ||
		errors.As(err, &hostErr) || errors.As(err, &sysRoots) || errors.As(err, &recordHeader) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:") || strings.Contains(msg, "certificate")
}

// IsTimeoutError reports whether err is a context deadline or network timeout error.
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// DescribeError translates transport and protocol errors into actionable mobile
// troubleshooting guidance without leaking sensitive token values.
func DescribeError(err error, origin string) string {
	if err == nil {
		return ""
	}
	switch {
	case IsAuthError(err):
		return "认证被拒绝（HTTP 401/403）：请检查 API Token 是否正确或已更新"
	case IsTLSError(err):
		return "TLS 证书验证失败：请确认 HTTPS 反向代理证书受系统信任且域名匹配"
	case IsTimeoutError(err):
		return "连接超时：请检查手机网络、VPN/反向代理可达性及后端端口"
	default:
		msg := err.Error()
		if IsLoopbackOrigin(origin) && strings.Contains(strings.ToLower(msg), "unreachable") {
			return "本机回环不可达：若在真机上通过 USB 调试，请先在电脑执行 adb reverse tcp:8080 tcp:8080"
		}
		return msg
	}
}
