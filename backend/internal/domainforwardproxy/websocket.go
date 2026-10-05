package domainforwardproxy

import (
	"bytes"
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

type responsesWSPendingSteer struct {
	mapping  *modelRewrite
	streamID string
}

type responsesWSRewriteState struct {
	mu               sync.RWMutex
	streamMappings   map[string][]*modelRewrite
	responseMappings map[string]*modelRewrite
	responseStreams  map[string]string
	responseOrder    []string
	pendingSteers    map[string][]responsesWSPendingSteer
}

func newResponsesWSRewriteState() *responsesWSRewriteState {
	return &responsesWSRewriteState{
		streamMappings:   make(map[string][]*modelRewrite),
		responseMappings: make(map[string]*modelRewrite),
		responseStreams:  make(map[string]string),
		pendingSteers:    make(map[string][]responsesWSPendingSteer),
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

func (s *responsesWSRewriteState) enqueueStream(streamID string, mapping *modelRewrite) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamMappings[streamID] = append(s.streamMappings[streamID], cloneModelRewrite(mapping))
}

func (s *responsesWSRewriteState) stream(streamID string) *modelRewrite {
	s.mu.RLock()
	defer s.mu.RUnlock()
	queue := s.streamMappings[streamID]
	if len(queue) == 0 {
		return nil
	}
	return cloneModelRewrite(queue[0])
}

func (s *responsesWSRewriteState) rememberResponse(responseID, streamID string, mapping *modelRewrite) {
	if responseID == "" || mapping == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.responseMappings[responseID]; !exists {
		s.responseOrder = append(s.responseOrder, responseID)
	}
	s.responseMappings[responseID] = cloneModelRewrite(mapping)
	s.responseStreams[responseID] = streamID
	for len(s.responseOrder) > maxResponsesWSRewriteHistory {
		oldest := s.responseOrder[0]
		s.responseOrder = s.responseOrder[1:]
		delete(s.responseMappings, oldest)
		delete(s.responseStreams, oldest)
		delete(s.pendingSteers, oldest)
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

func (s *responsesWSRewriteState) streamForResponse(responseID string) string {
	if responseID == "" {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.responseStreams[responseID]
}

func (s *responsesWSRewriteState) enqueueSteer(previousResponseID string, mapping *modelRewrite) {
	if previousResponseID == "" || mapping == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingSteers[previousResponseID] = append(
		s.pendingSteers[previousResponseID],
		responsesWSPendingSteer{
			mapping:  cloneModelRewrite(mapping),
			streamID: s.responseStreams[previousResponseID],
		},
	)
}

func (s *responsesWSRewriteState) hasPendingSteer(previousResponseID string) bool {
	if previousResponseID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.pendingSteers[previousResponseID]) > 0
}

// bindPendingSteerContinuation attaches an explicit response.create to steering
// input already owned by the server. A pending steer keeps the parent lane
// mapping queued, so appending another stream mapping here would leave stale
// state after the successor completes.
func (s *responsesWSRewriteState) bindPendingSteerContinuation(
	previousResponseID, streamID string,
	mapping *modelRewrite,
) bool {
	if previousResponseID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.pendingSteers[previousResponseID]
	if len(queue) == 0 {
		return false
	}
	for i := range queue {
		queue[i].mapping = cloneModelRewrite(mapping)
		queue[i].streamID = streamID
	}
	s.pendingSteers[previousResponseID] = queue
	return true
}

func (s *responsesWSRewriteState) takePendingSteer(previousResponseID string) (responsesWSPendingSteer, bool) {
	if previousResponseID == "" {
		return responsesWSPendingSteer{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.pendingSteers[previousResponseID]
	if len(queue) == 0 {
		return responsesWSPendingSteer{}, false
	}
	entry := queue[0]
	if len(queue) == 1 {
		delete(s.pendingSteers, previousResponseID)
	} else {
		queue[0] = responsesWSPendingSteer{}
		s.pendingSteers[previousResponseID] = queue[1:]
	}
	entry.mapping = cloneModelRewrite(entry.mapping)
	return entry, true
}

// takePendingSteerBatch transfers all steering submissions queued against one
// response into the single automatic successor response. The service may
// accept more than one response.steer before it creates that successor; leaving
// extra entries behind would make a later continuation inherit stale state.
func (s *responsesWSRewriteState) takePendingSteerBatch(previousResponseID string) (responsesWSPendingSteer, bool) {
	if previousResponseID == "" {
		return responsesWSPendingSteer{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.pendingSteers[previousResponseID]
	if len(queue) == 0 {
		return responsesWSPendingSteer{}, false
	}
	entry := queue[0]
	delete(s.pendingSteers, previousResponseID)
	entry.mapping = cloneModelRewrite(entry.mapping)
	return entry, true
}

func (s *responsesWSRewriteState) dropPendingSteer(previousResponseID string) (responsesWSPendingSteer, bool) {
	return s.takePendingSteer(previousResponseID)
}

func (s *responsesWSRewriteState) popStream(streamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.streamMappings[streamID]
	if len(queue) <= 1 {
		delete(s.streamMappings, streamID)
		return
	}
	queue[0] = nil
	s.streamMappings[streamID] = queue[1:]
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
				Type               string          `json:"type"`
				StreamID           string          `json:"stream_id"`
				PreviousResponseID string          `json:"previous_response_id"`
				Model              json.RawMessage `json:"model"`
			}
			if json.Unmarshal(payload, &envelope) == nil {
				switch envelope.Type {
				case "response.create":
					rewritten, mapping, _ := h.rewrite.RewriteRequest(host, path, "application/json", payload)
					payload = rewritten
					explicitModel := len(strings.TrimSpace(string(envelope.Model))) > 0 &&
						string(bytes.TrimSpace(envelope.Model)) != "null"
					if mapping == nil && !explicitModel {
						mapping = state.response(envelope.PreviousResponseID)
					}
					if !state.bindPendingSteerContinuation(
						envelope.PreviousResponseID,
						envelope.StreamID,
						mapping,
					) {
						state.enqueueStream(envelope.StreamID, mapping)
					}
				case "response.steer":
					rewritten, _, _ := h.rewrite.RewriteRequest(host, path, "application/json", payload)
					payload = rewritten
					state.enqueueSteer(envelope.PreviousResponseID, state.response(envelope.PreviousResponseID))
				default:
					rewritten, _, _ := h.rewrite.RewriteRequest(host, path, "application/json", payload)
					payload = rewritten
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
				ResponseID         string `json:"response_id"`
				PreviousResponseID string `json:"previous_response_id"`
				Response           struct {
					ID                 string `json:"id"`
					PreviousResponseID string `json:"previous_response_id"`
					IncompleteDetails  struct {
						Reason string `json:"reason"`
					} `json:"incomplete_details"`
				} `json:"response"`
				Steer struct {
					PreviousResponseID string `json:"previous_response_id"`
				} `json:"steer"`
				Error struct {
					Param string `json:"param"`
				} `json:"error"`
			}
			_ = json.Unmarshal(payload, &envelope)
			responseID := envelope.ResponseID
			if responseID == "" {
				responseID = envelope.Response.ID
			}
			previousResponseID := envelope.PreviousResponseID
			if previousResponseID == "" {
				previousResponseID = envelope.Response.PreviousResponseID
			}

			mapping := state.stream(envelope.StreamID)
			if envelope.Type == "response.created" && previousResponseID != "" {
				if steer, ok := state.takePendingSteerBatch(previousResponseID); ok {
					mapping = steer.mapping
				}
			}
			if mapping == nil && responseID != "" {
				mapping = state.response(responseID)
			}
			if mapping == nil && previousResponseID != "" {
				mapping = state.response(previousResponseID)
			}
			if mapping != nil && responseID != "" {
				state.rememberResponse(responseID, envelope.StreamID, mapping)
			}
			rewritten, _ := h.rewrite.RewriteResponse(host, path, "application/json", payload, mapping)
			payload = rewritten

			if envelope.Type == "response.steer.failed" {
				if failed, ok := state.dropPendingSteer(envelope.Steer.PreviousResponseID); ok {
					state.popStream(failed.streamID)
				}
			}
			if responsesTerminalEvent(envelope.Type, envelope.StreamID, envelope.Error.Param) &&
				!state.hasPendingSteer(responseID) {
				state.popStream(envelope.StreamID)
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

func responsesTerminalEvent(eventType, streamID, errorParam string) bool {
	switch eventType {
	case "response.completed", "response.failed", "response.incomplete":
		return true
	case "error":
		// A valid named-lane request error carries stream_id. Default-lane
		// request errors do not. Errors rejecting stream_id itself cannot be
		// associated with a lane, so do not pop the implicit default queue.
		return streamID != "" || errorParam != "stream_id"
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
