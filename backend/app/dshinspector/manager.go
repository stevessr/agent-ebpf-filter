package dshinspector

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultStartPort = 9230
	defaultEndPort   = 9240
)

type Event struct {
	Type        string
	RequestID   string
	Timestamp   time.Time
	Method      string
	URL         string
	StatusCode  int
	Headers     map[string]string
	Body        string
	BodySize    int
	ContentType string
	LatencyMs   float64
	Error       string
	SSEEvent    string
}

type Status struct {
	Connected bool   `json:"connected"`
	Endpoint  string `json:"endpoint,omitempty"`
	Port      int    `json:"port,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

type Sink func(Event)

type Manager struct {
	mu        sync.RWMutex
	session   *session
	endpoint  string
	port      int
	lastError string
	sink      Sink
}

func NewManager(sink Sink) *Manager {
	return &Manager{sink: sink}
}

func (m *Manager) Status() Status {
	if m == nil {
		return Status{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Status{
		Connected: m.session != nil,
		Endpoint:  m.endpoint,
		Port:      m.port,
		LastError: m.lastError,
	}
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	current := m.session
	m.session = nil
	m.endpoint = ""
	m.port = 0
	m.mu.Unlock()
	if current != nil {
		current.close()
	}
}

func (m *Manager) DiscoverAndConnect(ctx context.Context, port int) error {
	if m == nil {
		return errors.New("dsh inspector manager is unavailable")
	}
	start, end := defaultStartPort, defaultEndPort
	if port > 0 {
		start, end = port, port
	}
	var errs []error
	for candidate := start; candidate <= end; candidate++ {
		endpoint, err := discoverEndpoint(ctx, candidate)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := m.connect(ctx, endpoint, candidate); err != nil {
			errs = append(errs, err)
			continue
		}
		return nil
	}
	err := errors.Join(errs...)
	if err == nil {
		err = errors.New("no DeepSeek Harness Inspector endpoint found")
	}
	m.mu.Lock()
	m.lastError = err.Error()
	m.mu.Unlock()
	return err
}

func discoverEndpoint(ctx context.Context, port int) (string, error) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/json/list", port), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("port %d: %w", port, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("port %d: inspector returned %s", port, resp.Status)
	}
	var targets []struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return "", fmt.Errorf("port %d: decode inspector target: %w", port, err)
	}
	for _, target := range targets {
		if err := validateLoopbackWebSocket(target.WebSocketDebuggerURL); err == nil {
			return target.WebSocketDebuggerURL, nil
		}
	}
	return "", fmt.Errorf("port %d: no loopback CDP target", port)
}

func validateLoopbackWebSocket(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return fmt.Errorf("unsupported CDP scheme %q", parsed.Scheme)
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("refusing non-loopback Inspector endpoint %q", raw)
	}
	return nil
}

func (m *Manager) connect(ctx context.Context, endpoint string, port int) error {
	if err := validateLoopbackWebSocket(endpoint); err != nil {
		return err
	}
	dialer := websocket.Dialer{HandshakeTimeout: time.Second}
	conn, _, err := dialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return fmt.Errorf("connect dsh inspector: %w", err)
	}
	next := newSession(conn, m.sink)
	if err := next.send("Network.enable", map[string]any{}, nil); err != nil {
		_ = conn.Close()
		return fmt.Errorf("enable dsh inspector Network domain: %w", err)
	}

	m.mu.Lock()
	previous := m.session
	m.session = next
	m.endpoint = endpoint
	m.port = port
	m.lastError = ""
	m.mu.Unlock()
	if previous != nil {
		previous.close()
	}
	go func() {
		err := next.readLoop()
		m.mu.Lock()
		if m.session == next {
			m.session = nil
			m.endpoint = ""
			m.port = 0
			if err != nil && !errors.Is(err, net.ErrClosed) {
				m.lastError = err.Error()
			}
		}
		m.mu.Unlock()
	}()
	return nil
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cdpMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *cdpError       `json:"error,omitempty"`
}

type requestMeta struct {
	Method    string
	URL       string
	Headers   map[string]string
	Timestamp time.Time
}

type responseMeta struct {
	RequestID   string
	Method      string
	URL         string
	StatusCode  int
	Headers     map[string]string
	ContentType string
	Timestamp   time.Time
	LatencyMs   float64
}

type session struct {
	conn      *websocket.Conn
	sink      Sink
	nextID    atomic.Int64
	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   map[int64]func(json.RawMessage, *cdpError)
	stateMu   sync.Mutex
	requests  map[string]requestMeta
	responses map[string]responseMeta
	closed    atomic.Bool
}

func newSession(conn *websocket.Conn, sink Sink) *session {
	return &session{
		conn:      conn,
		sink:      sink,
		pending:   make(map[int64]func(json.RawMessage, *cdpError)),
		requests:  make(map[string]requestMeta),
		responses: make(map[string]responseMeta),
	}
}

func (s *session) close() {
	if s == nil || !s.closed.CompareAndSwap(false, true) {
		return
	}
	_ = s.conn.Close()
}

func (s *session) send(method string, params any, callback func(json.RawMessage, *cdpError)) error {
	id := s.nextID.Add(1)
	payload := map[string]any{"id": id, "method": method, "params": params}
	if callback != nil {
		s.pendingMu.Lock()
		s.pending[id] = callback
		s.pendingMu.Unlock()
	}
	s.writeMu.Lock()
	err := s.conn.WriteJSON(payload)
	s.writeMu.Unlock()
	if err != nil && callback != nil {
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
	}
	return err
}

func (s *session) readLoop() error {
	defer s.close()
	for {
		_, payload, err := s.conn.ReadMessage()
		if err != nil {
			return err
		}
		var message cdpMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			continue
		}
		if message.ID != 0 {
			s.pendingMu.Lock()
			callback := s.pending[message.ID]
			delete(s.pending, message.ID)
			s.pendingMu.Unlock()
			if callback != nil {
				callback(message.Result, message.Error)
			}
			continue
		}
		s.handleEvent(message.Method, message.Params)
	}
}

func (s *session) emit(event Event) {
	if s.sink != nil {
		s.sink(event)
	}
}

func headersOf(value map[string]any) map[string]string {
	if len(value) == 0 {
		return nil
	}
	out := make(map[string]string, len(value))
	for key, item := range value {
		out[key] = fmt.Sprint(item)
	}
	return out
}

func wallTime(seconds float64) time.Time {
	if seconds <= 0 {
		return time.Now().UTC()
	}
	whole := int64(seconds)
	nanos := int64((seconds - float64(whole)) * float64(time.Second))
	return time.Unix(whole, nanos).UTC()
}

func (s *session) handleEvent(method string, raw json.RawMessage) {
	switch method {
	case "Network.requestWillBeSent":
		var params struct {
			RequestID string  `json:"requestId"`
			WallTime  float64 `json:"wallTime"`
			Request   struct {
				URL         string         `json:"url"`
				Method      string         `json:"method"`
				Headers     map[string]any `json:"headers"`
				PostData    string         `json:"postData"`
				HasPostData bool           `json:"hasPostData"`
			} `json:"request"`
		}
		if json.Unmarshal(raw, &params) != nil || params.RequestID == "" {
			return
		}
		meta := requestMeta{
			Method: params.Request.Method, URL: params.Request.URL,
			Headers: headersOf(params.Request.Headers), Timestamp: wallTime(params.WallTime),
		}
		s.stateMu.Lock()
		s.requests[params.RequestID] = meta
		s.stateMu.Unlock()
		emit := func(body string) {
			s.emit(Event{
				Type: "http_request", RequestID: params.RequestID, Timestamp: meta.Timestamp,
				Method: meta.Method, URL: meta.URL, Headers: meta.Headers,
				Body: body, BodySize: len([]byte(body)),
			})
		}
		if params.Request.PostData != "" || !params.Request.HasPostData {
			emit(params.Request.PostData)
			return
		}
		_ = s.send("Network.getRequestPostData", map[string]any{"requestId": params.RequestID}, func(result json.RawMessage, cdpErr *cdpError) {
			if cdpErr != nil {
				emit("")
				return
			}
			var value struct {
				PostData string `json:"postData"`
			}
			if json.Unmarshal(result, &value) != nil {
				emit("")
				return
			}
			emit(value.PostData)
		})
	case "Network.responseReceived":
		var params struct {
			RequestID string  `json:"requestId"`
			Timestamp float64 `json:"timestamp"`
			Response  struct {
				URL      string         `json:"url"`
				Status   float64        `json:"status"`
				Headers  map[string]any `json:"headers"`
				MimeType string         `json:"mimeType"`
			} `json:"response"`
		}
		if json.Unmarshal(raw, &params) != nil || params.RequestID == "" {
			return
		}
		s.stateMu.Lock()
		req := s.requests[params.RequestID]
		s.responses[params.RequestID] = responseMeta{
			RequestID: params.RequestID, Method: req.Method,
			URL: params.Response.URL, StatusCode: int(params.Response.Status),
			Headers: headersOf(params.Response.Headers), ContentType: params.Response.MimeType,
			Timestamp: time.Now().UTC(),
			LatencyMs: float64(time.Since(req.Timestamp).Microseconds()) / 1000,
		}
		s.stateMu.Unlock()
	case "Network.loadingFinished":
		var params struct {
			RequestID string `json:"requestId"`
		}
		if json.Unmarshal(raw, &params) != nil || params.RequestID == "" {
			return
		}
		s.finishResponse(params.RequestID)
	case "Network.loadingFailed":
		var params struct {
			RequestID string `json:"requestId"`
			ErrorText string `json:"errorText"`
		}
		if json.Unmarshal(raw, &params) != nil || params.RequestID == "" {
			return
		}
		s.stateMu.Lock()
		req := s.requests[params.RequestID]
		delete(s.requests, params.RequestID)
		delete(s.responses, params.RequestID)
		s.stateMu.Unlock()
		s.emit(Event{
			Type: "http_error", RequestID: params.RequestID, Timestamp: time.Now().UTC(),
			Method: req.Method, URL: req.URL, Error: params.ErrorText,
		})
	case "Network.eventSourceMessageReceived":
		var params struct {
			RequestID string `json:"requestId"`
			EventName string `json:"eventName"`
			Data      string `json:"data"`
		}
		if json.Unmarshal(raw, &params) != nil || params.RequestID == "" {
			return
		}
		s.stateMu.Lock()
		req := s.requests[params.RequestID]
		resp := s.responses[params.RequestID]
		s.stateMu.Unlock()
		targetURL := resp.URL
		if targetURL == "" {
			targetURL = req.URL
		}
		s.emit(Event{
			Type: "sse_message", RequestID: params.RequestID, Timestamp: time.Now().UTC(),
			Method: req.Method, URL: targetURL, StatusCode: resp.StatusCode,
			Headers: resp.Headers, ContentType: "text/event-stream",
			Body: params.Data, BodySize: len([]byte(params.Data)), SSEEvent: params.EventName,
		})
	}
}

func (s *session) finishResponse(requestID string) {
	s.stateMu.Lock()
	meta, ok := s.responses[requestID]
	delete(s.responses, requestID)
	delete(s.requests, requestID)
	s.stateMu.Unlock()
	if !ok {
		return
	}
	emit := func(body string) {
		s.emit(Event{
			Type: "http_response", RequestID: requestID, Timestamp: meta.Timestamp,
			Method: meta.Method, URL: meta.URL, StatusCode: meta.StatusCode,
			Headers: meta.Headers, Body: body, BodySize: len([]byte(body)),
			ContentType: meta.ContentType, LatencyMs: meta.LatencyMs,
		})
	}
	_ = s.send("Network.getResponseBody", map[string]any{"requestId": requestID}, func(result json.RawMessage, cdpErr *cdpError) {
		if cdpErr != nil {
			emit("")
			return
		}
		var value struct {
			Body          string `json:"body"`
			Base64Encoded bool   `json:"base64Encoded"`
		}
		if json.Unmarshal(result, &value) != nil {
			emit("")
			return
		}
		if value.Base64Encoded {
			decoded, err := base64.StdEncoding.DecodeString(value.Body)
			if err == nil {
				value.Body = string(decoded)
			}
		}
		emit(value.Body)
	})
}

func ParsePort(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid inspector port %q", raw)
	}
	return port, nil
}
