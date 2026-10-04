package domainforwardproxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const websocketWriteTimeout = 30 * time.Second

type responsesWSRewriteState struct {
	mu       sync.RWMutex
	mappings map[string]*modelRewrite
}

func newResponsesWSRewriteState() *responsesWSRewriteState {
	return &responsesWSRewriteState{mappings: make(map[string]*modelRewrite)}
}

func (s *responsesWSRewriteState) set(streamID string, mapping *modelRewrite) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mapping == nil {
		delete(s.mappings, streamID)
		return
	}
	copy := *mapping
	s.mappings[streamID] = &copy
}

func (s *responsesWSRewriteState) get(streamID string) *modelRewrite {
	s.mu.RLock()
	defer s.mu.RUnlock()
	mapping := s.mappings[streamID]
	if mapping == nil {
		return nil
	}
	copy := *mapping
	return &copy
}

func (s *responsesWSRewriteState) clear(streamID string) {
	s.mu.Lock()
	delete(s.mappings, streamID)
	s.mu.Unlock()
}

func isResponsesWebSocketRequest(r *http.Request) bool {
	if r == nil || r.URL == nil || !websocket.IsWebSocketUpgrade(r) {
		return false
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	return path == "/v1/responses" || strings.HasSuffix(path, "/v1/responses")
}

func (h *Handler) serveResponsesWebSocket(
	w http.ResponseWriter,
	r *http.Request,
	target *url.URL,
	route DomainForwardRoute,
	host string,
) {
	upstreamURL := websocketTargetURL(target, r.URL)
	headers := r.Header.Clone()
	subprotocols := websocket.Subprotocols(r)
	for _, name := range []string{
		"Connection",
		"Upgrade",
		"Sec-WebSocket-Key",
		"Sec-WebSocket-Version",
		"Sec-WebSocket-Extensions",
		"Sec-WebSocket-Protocol",
	} {
		headers.Del(name)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		EnableCompression: true,
		Subprotocols:      subprotocols,
	}
	if transport, ok := h.transport.(*http.Transport); ok {
		dialer.NetDialContext = transport.DialContext
		if transport.TLSClientConfig != nil {
			dialer.TLSClientConfig = transport.TLSClientConfig.Clone()
		} else {
			dialer.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}
	}

	upstream, response, err := dialer.DialContext(r.Context(), upstreamURL.String(), headers)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
		}
		http.Error(w, "upstream websocket handshake failed", http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	upgrader := websocket.Upgrader{
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     nonEmptyString(upstream.Subprotocol()),
		CheckOrigin: func(request *http.Request) bool {
			origin := strings.TrimSpace(request.Header.Get("Origin"))
			if origin == "" {
				return true
			}
			parsed, err := url.Parse(origin)
			return err == nil && NormalizeForwardHost(parsed.Host) == NormalizeForwardHost(request.Host)
		},
	}
	client, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer client.Close()

	state := newResponsesWSRewriteState()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	errs := make(chan error, 2)
	go func() {
		errs <- h.copyResponsesWSClientToUpstream(ctx, upstream, client, state, route, host, r.URL.Path)
	}()
	go func() {
		errs <- h.copyResponsesWSUpstreamToClient(ctx, client, upstream, state, route, host, r.URL.Path)
	}()

	<-errs
	cancel()
	_ = client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	_ = upstream.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
}

func (h *Handler) copyResponsesWSClientToUpstream(
	ctx context.Context,
	dst, src *websocket.Conn,
	state *responsesWSRewriteState,
	route DomainForwardRoute,
	host, path string,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		messageType, payload, err := src.ReadMessage()
		if err != nil {
			return err
		}
		if messageType == websocket.TextMessage {
			var envelope struct {
				Type     string `json:"type"`
				StreamID string `json:"stream_id"`
			}
			if json.Unmarshal(payload, &envelope) == nil && envelope.Type == "response.create" {
				rewritten, mapping, _ := h.rewrite.RewriteRequest(host, path, "application/json", payload)
				payload = rewritten
				if mapping != nil {
					state.set(envelope.StreamID, mapping)
				}
			} else {
				rewritten, _, _ := h.rewrite.RewriteRequest(host, path, "application/json", payload)
				payload = rewritten
			}
		}
		_ = dst.SetWriteDeadline(time.Now().Add(websocketWriteTimeout))
		if err := dst.WriteMessage(messageType, payload); err != nil {
			return err
		}
	}
}

func (h *Handler) copyResponsesWSUpstreamToClient(
	ctx context.Context,
	dst, src *websocket.Conn,
	state *responsesWSRewriteState,
	route DomainForwardRoute,
	host, path string,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		messageType, payload, err := src.ReadMessage()
		if err != nil {
			return err
		}
		if messageType == websocket.TextMessage {
			var envelope struct {
				Type     string `json:"type"`
				StreamID string `json:"stream_id"`
			}
			_ = json.Unmarshal(payload, &envelope)
			rewritten, _ := h.rewrite.RewriteResponse(host, path, "application/json", payload, state.get(envelope.StreamID))
			payload = rewritten
			if responsesTerminalEvent(envelope.Type) {
				state.clear(envelope.StreamID)
			}
		}
		_ = dst.SetWriteDeadline(time.Now().Add(websocketWriteTimeout))
		if err := dst.WriteMessage(messageType, payload); err != nil {
			return err
		}
	}
}

func websocketTargetURL(target, incoming *url.URL) *url.URL {
	out := *target
	switch strings.ToLower(out.Scheme) {
	case "http":
		out.Scheme = "ws"
	default:
		out.Scheme = "wss"
	}
	out.Path, out.RawPath = JoinPath(target, incoming)
	switch {
	case target.RawQuery == "":
		out.RawQuery = incoming.RawQuery
	case incoming.RawQuery == "":
		out.RawQuery = target.RawQuery
	default:
		out.RawQuery = target.RawQuery + "&" + incoming.RawQuery
	}
	return &out
}

func responsesTerminalEvent(eventType string) bool {
	switch eventType {
	case "response.completed", "response.failed", "response.incomplete":
		return true
	default:
		return false
	}
}

func nonEmptyString(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

// drainAndClose is kept small so future handshake diagnostics can consume a
// bounded upstream error body without exposing provider details to clients.
func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4096))
	_ = body.Close()
}

var _ = errors.Is
