package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const tlsMaxBodySize = 16 * 1024

func parseTLSPlaintext(fragment completedTLSFragment) TLSPlaintextEvent {
	event := TLSPlaintextEvent{
		Type:         "raw",
		Timestamp:    time.Unix(0, int64(fragment.TimestampNS)).UTC(),
		PID:          fragment.PID,
		TGID:         fragment.TGID,
		Comm:         fragment.Comm,
		Direction:    tlsDirectionLabel(fragment.Direction),
		Lib:          tlsLibLabel(fragment.LibType),
		RawAvailable: false,
	}

	if req, ok := parseTLSPlaintextHTTPRequest(fragment.Payload); ok {
		return buildTLSPlaintextHTTPRequestEvent(event, req)
	}
	if resp, ok := parseTLSPlaintextHTTPResponse(fragment.Payload); ok {
		return buildTLSPlaintextHTTPResponseEvent(event, resp)
	}

	event.RawHexDump = hexDump(fragment.Payload)
	return event
}

type tlsHTTPRequest struct {
	req      *http.Request
	body     []byte
	bodySize int
	host     string
	content  string
}

type tlsHTTPResponse struct {
	resp     *http.Response
	body     []byte
	bodySize int
	content  string
}

func parseTLSPlaintextHTTPRequest(payload []byte) (*tlsHTTPRequest, bool) {
	if !looksLikeTLSHTTPRequest(payload) {
		return nil, false
	}

	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(payload)))
	if err != nil {
		return nil, false
	}
	body, bodySize, err := readBoundedTLSBody(req.Body)
	if err != nil {
		_ = req.Body.Close()
		return nil, false
	}
	_ = req.Body.Close()
	contentType := req.Header.Get("Content-Type")
	if contentType == "" {
		contentType = req.Header.Get("content-type")
	}
	return &tlsHTTPRequest{
		req:      req,
		body:     body,
		bodySize: bodySize,
		host:     req.Host,
		content:  contentType,
	}, true
}

func looksLikeTLSHTTPRequest(payload []byte) bool {
	lineEnd := bytes.IndexAny(payload, "\r\n")
	if lineEnd <= 0 {
		return false
	}
	firstLine := string(payload[:lineEnd])
	parts := strings.Split(firstLine, " ")
	if len(parts) != 3 {
		return false
	}
	if !validTLSHTTPRequestMethod(parts[0]) {
		return false
	}
	if parts[1] == "" {
		return false
	}
	if !strings.HasPrefix(parts[2], "HTTP/1.") {
		return false
	}
	return true
}

func validTLSHTTPRequestMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions, http.MethodTrace, http.MethodConnect:
		return true
	default:
		return false
	}
}

func parseTLSPlaintextHTTPResponse(payload []byte) (*tlsHTTPResponse, bool) {
	if !bytes.HasPrefix(payload, []byte("HTTP/")) {
		return nil, false
	}
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(payload)), &http.Request{Method: http.MethodGet})
	if err != nil {
		return nil, false
	}
	body, bodySize, err := readBoundedTLSBody(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return nil, false
	}
	_ = resp.Body.Close()
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = resp.Header.Get("content-type")
	}
	return &tlsHTTPResponse{
		resp:     resp,
		body:     body,
		bodySize: bodySize,
		content:  contentType,
	}, true
}

func buildTLSPlaintextHTTPRequestEvent(base TLSPlaintextEvent, parsed *tlsHTTPRequest) TLSPlaintextEvent {
	base.Type = "http_request"
	base.Method = parsed.req.Method
	base.URL = parsed.req.URL.String()
	base.Host = parsed.host
	base.Headers = sanitizeTLSHeaders(parsed.req.Header)
	base.ContentType = parsed.content
	base.BodySize = parsed.bodySize
	base.Body, base.Truncated = formatTLSPlaintextBody(parsed.body, base.ContentType)
	base.RawAvailable = true
	return base
}

func buildTLSPlaintextHTTPResponseEvent(base TLSPlaintextEvent, parsed *tlsHTTPResponse) TLSPlaintextEvent {
	base.Type = "http_response"
	base.StatusCode = parsed.resp.StatusCode
	base.Headers = sanitizeTLSHeaders(parsed.resp.Header)
	base.ContentType = parsed.content
	base.BodySize = parsed.bodySize
	base.Body, base.Truncated = formatTLSPlaintextBody(parsed.body, base.ContentType)
	base.RawAvailable = true
	return base
}

func sanitizeTLSHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	redacted := map[string]string{
		"authorization":       "***REDACTED***",
		"proxy-authorization": "***REDACTED***",
		"x-api-key":           "***REDACTED***",
		"cookie":              "***REDACTED***",
		"set-cookie":          "***REDACTED***",
	}
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		lower := strings.ToLower(key)
		if replacement, ok := redacted[lower]; ok {
			out[lower] = replacement
			continue
		}
		out[lower] = strings.Join(values, ", ")
	}
	return out
}

func formatTLSPlaintextBody(body []byte, contentType string) (string, bool) {
	if len(body) == 0 {
		return "", false
	}

	formatted := body
	if looksLikeTLSJSON(contentType, body) {
		if pretty, err := prettyPrintJSON(body); err == nil {
			formatted = pretty
		}
	}

	truncated := len(body) > tlsMaxBodySize || len(formatted) > tlsMaxBodySize
	if len(formatted) > tlsMaxBodySize {
		formatted = formatted[:tlsMaxBodySize]
	}
	return string(formatted), truncated
}

func readBoundedTLSBody(r io.Reader) ([]byte, int, error) {
	limited := io.LimitReader(r, tlsMaxBodySize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, 0, err
	}
	return body, len(body), nil
}

func looksLikeTLSJSON(contentType string, body []byte) bool {
	lower := strings.ToLower(contentType)
	if strings.Contains(lower, "json") || strings.Contains(lower, "+json") {
		return json.Valid(bytes.TrimSpace(body))
	}
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(trimmed)
}

func prettyPrintJSON(body []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func hexDump(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var b strings.Builder
	for i, v := range payload {
		if i > 0 {
			b.WriteByte(' ')
		}
		_, _ = fmt.Fprintf(&b, "%02x", v)
	}
	return b.String()
}

func tlsDirectionLabel(direction uint8) string {
	switch direction {
	case tlsDirectionRecv:
		return "recv"
	case tlsDirectionSend:
		return "send"
	default:
		return fmt.Sprintf("direction_%d", direction)
	}
}

func tlsLibLabel(lib uint8) string {
	switch lib {
	case tlsLibOpenSSL:
		return "openssl"
	case tlsLibGo:
		return "go"
	case tlsLibGnuTLS:
		return "gnutls"
	case tlsLibNSS:
		return "nss"
	default:
		return fmt.Sprintf("lib_%d", lib)
	}
}

