package domainforwardproxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const websocketWriteTimeout = 30 * time.Second

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
						if inherited, known := state.response(envelope.PreviousResponseID); known {
							mapping = inherited
						}
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
					if mapping, known := state.response(envelope.PreviousResponseID); known {
						state.enqueueSteer(envelope.PreviousResponseID, mapping)
					}
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
					ID                 string `json:"id"`
					PreviousResponseID string `json:"previous_response_id"`
				} `json:"steer"`
				Error struct {
					Code  string `json:"code"`
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

			mapping, mappingBound := state.stream(envelope.StreamID)
			if envelope.Type == "response.created" && previousResponseID != "" {
				if steer, ok := state.takePendingSteerBatch(previousResponseID); ok {
					mapping = steer.mapping
					mappingBound = true
				}
			}
			if !mappingBound && responseID != "" {
				if knownMapping, known := state.response(responseID); known {
					mapping = knownMapping
					mappingBound = true
				}
			}
			if !mappingBound && previousResponseID != "" {
				if inherited, known := state.response(previousResponseID); known {
					mapping = inherited
					mappingBound = true
				}
			}
			if mappingBound && responseID != "" {
				state.rememberResponse(responseID, envelope.StreamID, mapping)
			}
			rewritten, _ := h.rewrite.RewriteResponse(host, path, "application/json", payload, mapping)
			payload = rewritten

			if envelope.Type == "response.steer.accepted" {
				state.acknowledgeSteer(envelope.Steer.PreviousResponseID, envelope.Steer.ID)
			}
			if envelope.Type == "response.steer.failed" {
				if _, ok := state.dropPendingSteer(
					envelope.Steer.PreviousResponseID,
					envelope.Steer.ID,
				); ok {
					if streamID, release := state.releaseHeldTerminalResponse(
						envelope.Steer.PreviousResponseID,
					); release {
						state.popStream(streamID)
					}
				}
			}
			if responsesTerminalEvent(
				envelope.Type,
				envelope.StreamID,
				envelope.Error.Param,
				envelope.Error.Code,
			) {
				if state.hasPendingSteer(responseID) {
					state.holdTerminalResponse(responseID, envelope.StreamID)
				} else {
					state.popStream(envelope.StreamID)
				}
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

func responsesTerminalEvent(eventType, streamID, errorParam, errorCode string) bool {
	switch eventType {
	case "response.completed", "response.failed", "response.incomplete":
		return true
	case "error":
		return responsesErrorTerminatesLane(streamID, errorParam, errorCode)
	default:
		return false
	}
}

func responsesErrorTerminatesLane(streamID, errorParam, errorCode string) bool {
	if streamID != "" {
		return true
	}
	switch errorCode {
	case "invalid_stream_id",
		"websocket_stream_limit_reached",
		"websocket_connection_limit_reached":
		return false
	case "previous_response_not_found":
		return true
	}
	if errorParam == "stream_id" {
		return false
	}
	// A parameter-specific error without a named stream belongs to the default
	// lane request. An unscoped unknown error is kept connection-level so it
	// cannot silently consume the default lane's queued model mapping.
	return errorParam != ""
}

func nonEmptyString(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}
