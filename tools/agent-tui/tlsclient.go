package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// TLSEvent mirrors the fields of the backend's TLSPlaintextEvent JSON that
// the monitor displays. Unknown fields are ignored so the TUI keeps working
// as the backend schema grows.
type TLSEvent struct {
	Type           string            `json:"type"`
	Timestamp      time.Time         `json:"timestamp"`
	PID            uint32            `json:"pid"`
	TGID           uint32            `json:"tgid"`
	Comm           string            `json:"comm"`
	Direction      string            `json:"direction"`
	Lib            string            `json:"lib"`
	Function       string            `json:"function,omitempty"`
	CapturedLen    int               `json:"captured_len"`
	OriginalLen    int               `json:"original_len"`
	Method         string            `json:"method,omitempty"`
	URL            string            `json:"url,omitempty"`
	Host           string            `json:"host,omitempty"`
	StatusCode     int               `json:"status,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Body           string            `json:"body,omitempty"`
	BodySize       int               `json:"body_size"`
	ContentType    string            `json:"content_type,omitempty"`
	Truncated      bool              `json:"truncated"`
	RedactionState string            `json:"redaction_state,omitempty"`
	SSEEvent       string            `json:"sse_event,omitempty"`
	HTTP2StreamID  uint32            `json:"http2_stream_id,omitempty"`
	HTTP2FrameType string            `json:"http2_frame_type,omitempty"`
	AgentRunID     string            `json:"agent_run_id,omitempty"`
	ToolName       string            `json:"tool_name,omitempty"`
	Vendor         string            `json:"vendor,omitempty"`
	MessageRole    string            `json:"message_role,omitempty"`
	PromptLen      int               `json:"prompt_len,omitempty"`
	LoopAlert      bool              `json:"loop_alert,omitempty"`
	IsHandshake    bool              `json:"is_handshake"`
	LatencyMs      float64           `json:"latency_ms,omitempty"`
}

// TLSHandler receives TLS capture stream callbacks.
type TLSHandler interface {
	OnTLSEvents(events []*TLSEvent)
	OnTLSHistory(events []*TLSEvent)
	OnTLSState(state ConnState, detail string)
}

// ErrTLSCaptureDisabled reports that the backend refused the subscription
// because TLS capture is switched off at runtime (HTTP 403).
var ErrTLSCaptureDisabled = errors.New("TLS capture is disabled on the backend")

// TLSStream keeps one subscription to /ws/tls-capture alive. Every frame is
// one JSON-encoded TLSEvent.
type TLSStream struct {
	cfg                    Config
	handler                TLSHandler
	dialer                 *websocket.Dialer
	http                   *http.Client
	minBackoff, maxBackoff time.Duration
}

func NewTLSStream(cfg Config, handler TLSHandler) *TLSStream {
	return &TLSStream{
		cfg:        cfg,
		handler:    handler,
		dialer:     &websocket.Dialer{HandshakeTimeout: 5 * time.Second},
		http:       &http.Client{Timeout: 10 * time.Second},
		minBackoff: time.Second,
		maxBackoff: 30 * time.Second,
	}
}

func (s *TLSStream) headers() http.Header {
	h := http.Header{}
	if s.cfg.Token != "" {
		h.Set("X-API-KEY", s.cfg.Token)
	}
	return h
}

func (s *TLSStream) wsURL() (string, error) {
	parsed, err := url.Parse(s.cfg.BackendURL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else {
		parsed.Scheme = "ws"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/ws/tls-capture"
	return parsed.String(), nil
}

// Run blocks until ctx is cancelled. A backend with TLS capture disabled is
// reported once and re-tried at the slow end of the backoff range.
func (s *TLSStream) Run(ctx context.Context) {
	backoff := s.minBackoff
	backfilled := false
	for {
		s.handler.OnTLSState(StateConnecting, "")
		if !backfilled && s.cfg.Backfill > 0 {
			if events, err := s.fetchHistory(ctx, s.cfg.Backfill); err == nil {
				s.handler.OnTLSHistory(events)
				backfilled = true
			}
		}
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			s.handler.OnTLSState(StateDisconnected, "stopped")
			return
		}
		if err == nil {
			err = errors.New("connection closed")
		}
		if errors.Is(err, ErrTLSCaptureDisabled) {
			backoff = s.maxBackoff
		}
		s.handler.OnTLSState(StateDisconnected, fmt.Sprintf("%v (retry in %s)", err, backoff))
		select {
		case <-ctx.Done():
			s.handler.OnTLSState(StateDisconnected, "stopped")
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, s.maxBackoff)
	}
}

func (s *TLSStream) runOnce(ctx context.Context) error {
	target, err := s.wsURL()
	if err != nil {
		return err
	}
	conn, resp, err := s.dialer.DialContext(ctx, target, s.headers())
	if err != nil {
		if resp != nil {
			if resp.StatusCode == http.StatusForbidden {
				return ErrTLSCaptureDisabled
			}
			return fmt.Errorf("%w (HTTP %d)", err, resp.StatusCode)
		}
		return err
	}
	defer conn.Close()
	s.handler.OnTLSState(StateConnected, target)

	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.TextMessage {
			continue
		}
		event := &TLSEvent{}
		if err := json.Unmarshal(data, event); err != nil {
			return fmt.Errorf("decode TLS event: %w", err)
		}
		s.handler.OnTLSEvents([]*TLSEvent{event})
	}
}

// fetchHistory loads recent TLS events from /tls-capture/recent (newest
// first on the wire) and returns them oldest first.
func (s *TLSStream) fetchHistory(ctx context.Context, limit int) ([]*TLSEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.BackendURL+"/tls-capture/recent?limit="+strconv.Itoa(limit), nil)
	if err != nil {
		return nil, err
	}
	req.Header = s.headers()
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/tls-capture/recent: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Events []*TLSEvent `json:"events"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode /tls-capture/recent: %w", err)
	}
	events := payload.Events
	if len(events) > 1 && events[0].Timestamp.After(events[len(events)-1].Timestamp) {
		for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
			events[i], events[j] = events[j], events[i]
		}
	}
	return events, nil
}
