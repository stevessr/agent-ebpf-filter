package tls

import (
	"net/http"
	"net/url"
	"strings"

	"agent-ebpf-filter/app/dshinspector"
)

type DshInspectorSink struct {
	Store       *TLSCaptureStore
	Broadcaster *TLSBroadcaster
}

func NewDshInspectorSink(store *TLSCaptureStore, broadcaster *TLSBroadcaster) DshInspectorSink {
	return DshInspectorSink{Store: store, Broadcaster: broadcaster}
}

func (s DshInspectorSink) Handle(event dshinspector.Event) {
	rawBody := []byte(event.Body)
	body, truncated := formatTLSPlaintextBody(rawBody, event.ContentType)
	body = sanitizeTLSBody(body, event.ContentType)
	sanitizedURL := sanitizeTLSURL(event.URL)
	parsed, _ := url.Parse(sanitizedURL)
	host := ""
	requestPath := ""
	if parsed != nil {
		host = sanitizeTLSInlineSecrets(parsed.Host)
		requestPath = parsed.EscapedPath()
		if requestPath == "" {
			requestPath = parsed.Path
		}
	}

	headers := make(http.Header, len(event.Headers))
	for key, value := range event.Headers {
		headers.Set(key, value)
	}
	contentType := event.ContentType
	if contentType == "" {
		contentType = headers.Get("Content-Type")
	}

	direction := "recv"
	if event.Type == "http_request" {
		direction = "send"
	}

	tlsEvent := TLSPlaintextEvent{
		Type:             event.Type,
		Timestamp:        event.Timestamp,
		Comm:             "dsh",
		Direction:        direction,
		Lib:              "dsh-inspector",
		Function:         "cdp.Network",
		CapturedLen:      len(rawBody),
		OriginalLen:      event.BodySize,
		Method:           event.Method,
		URL:              sanitizedURL,
		Host:             host,
		StatusCode:       event.StatusCode,
		Headers:          sanitizeTLSHeaders(headers),
		Body:             body,
		BodySize:         event.BodySize,
		ContentType:      contentType,
		RawAvailable:     event.Body != "",
		Truncated:        truncated,
		RedactionState:   "sanitized",
		Vendor:           "deepseek",
		CaptureSource:    "dsh_inspector",
		CaptureRequestID: event.RequestID,
		AppProtocol:      "http",
		RequestPath:      requestPath,
		LatencyMs:        event.LatencyMs,
		DataType:         "inspector",
		SSEEvent:         event.SSEEvent,
	}
	if event.Type == "sse_message" {
		tlsEvent.AppProtocol = "sse"
		tlsEvent.SSEDataCount = 1
	}
	if event.Error != "" {
		tlsEvent.Body = sanitizeTLSInlineSecrets(event.Error)
		tlsEvent.BodySize = len(event.Error)
		tlsEvent.DataType = "inspector_error"
	}
	if strings.TrimSpace(tlsEvent.Timestamp.String()) == "" {
		// Timestamp is supplied by the Inspector manager; keep this defensive
		// branch unreachable in normal operation rather than inventing clock data.
		return
	}

	DispatchTLSAgentEvent(&tlsEvent, tlsAgentLoopDetector, deps.Broadcast)
	if s.Store != nil {
		s.Store.Add(tlsEvent)
	}
	if s.Broadcaster != nil {
		s.Broadcaster.Broadcast(tlsEvent)
	}
}
