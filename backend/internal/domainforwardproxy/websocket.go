package domainforwardproxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const websocketWriteTimeout = 30 * time.Second

const maxResponsesWSRewriteHistory = 256

type responsesWSRewriteState struct {
	mu               sync.RWMutex
	streamMappings   map[string]*modelRewrite
	responseMappings map[string]*modelRewrite
	responseOrder    []string
}

func newResponsesWSRewriteState() *responsesWSRewriteState {
	return &responsesWSRewriteState{
		streamMappings:   make(map[string]*modelRewrite),
		responseMappings: make(map[string]*modelRewrite),
	}
}

func cloneModelRewrite(mapping *modelRewrite) *modelRewrite {
	if mapping == nil {
		return nil
	}
	copy := *mapping
	copy.clientJSON = append([]byte(nil), mapping.clientJSON...)
	copy.upstreamJSON = append([]byte(nil), mapping.upstreamJSON...)
	return &copy
}

func (s *responsesWSRewriteState) setStream(streamID string, mapping *modelRewrite) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mapping == nil {
		delete(s.streamMappings, streamID)
		return
	}
	s.streamMappings[streamID] = cloneModelRewrite(mapping)
}

func (s *responsesWSRewriteState) stream(streamID string) *modelRewrite {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneModelRewrite(s.streamMappings[streamID])
}

func (s *responsesWSRewriteState) rememberResponse(responseID string, mapping *modelRewrite) {
	if responseID == "" || mapping == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.responseMappings[responseID]; !exists {
		s.responseOrder = append(s.responseOrder, responseID)
	}
	s.responseMappings[responseID] = cloneModelRewrite(mapping)
	for len(s.responseOrder) > maxResponsesWSRewriteHistory {
		oldest := s.responseOrder[0]
		s.responseOrder = s.responseOrder[1:]
		delete(s.responseMappings, oldest)
	}
}

func (s *responsesWSRewriteState) response(responseID string) *modelRewrite {
	if responseID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneModelRewrite(s.responseMappings[responseID])
}

func (s *responsesWSRewriteState) clearStream(streamID string) {
	s.mu.Lock()
	delete(s.streamMappings, streamID)
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
		HandshakeTimeout:  10 * time.Second,
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

	readLimit := h.rewrite.MaxBodyBytes()
	if readLimit < 8<<20 {
		readLimit = 8 << 20
	}
	if readLimit > 64<<20 {
		readLimit = 64 << 20
	}
	client.SetReadLimit(readLimit)
	upstream.SetReadLimit(readLimit)

	state := newResponsesWSRewriteState()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	errs := make(chan error, 2)
	go func() {
		errs <- h.copyResponsesWSClientToUpstream(ctx, upstream, client, state, host, r.URL.Path)
	}()
	go func() {
		errs <- h.copyResponsesWSUpstreamToClient(ctx, client, upstream, state, host, r.URL.Path)
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
				Type               string `json:"type"`
				StreamID           string `json:"stream_id"`
				PreviousResponseID string `json:"previous_response_id"`
			}
			if json.Unmarshal(payload, &envelope) == nil && envelope.Type == "response.create" {
				rewritten, mapping, _ := h.rewrite.RewriteRequest(host, path, "application/json", payload)
				payload = rewritten
				if mapping == nil {
					mapping = state.response(envelope.PreviousResponseID)
				}
				state.setStream(envelope.StreamID, mapping)
			} else {
				rewritten, _, _ := h.rewrite.RewriteRequest(host, path, "application/json", payload)
				payload = rewritten
				if envelope.Type == "response.cancel" {
					state.clearStream(envelope.StreamID)
				}
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
				Type       string `json:"type"`
				StreamID   string `json:"stream_id"`
				ResponseID string `json:"response_id"`
				Response   struct {
					ID string `json:"id"`
				} `json:"response"`
			}
			_ = json.Unmarshal(payload, &envelope)
			mapping := state.stream(envelope.StreamID)
			responseID := envelope.ResponseID
			if responseID == "" {
				responseID = envelope.Response.ID
			}
			if mapping != nil && responseID != "" {
				state.rememberResponse(responseID, mapping)
			}
			rewritten, _ := h.rewrite.RewriteResponse(host, path, "application/json", payload, mapping)
			payload = rewritten
			if responsesTerminalEvent(envelope.Type) {
				state.clearStream(envelope.StreamID)
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
