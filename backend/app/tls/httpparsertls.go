package tls

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section httpparsertls.go ----

const tlsMaxBodySize = 16 * 1024

const tlsRedactedValue = "***REDACTED***"

var tlsSensitiveQueryKeys = map[string]struct{}{
	"access_token":  {},
	"api_key":       {},
	"apikey":        {},
	"auth":          {},
	"authorization": {},
	"bearer":        {},
	"client_secret": {},
	"key":           {},
	"password":      {},
	"secret":        {},
	"session":       {},
	"token":         {},
}

var tlsBearerTokenPattern = regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9._~+/=-]+`)
var tlsInlineSecretPattern = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|auth[_-]?token|client[_-]?secret|password|secret|token)=([^\s&]+)`)

func parseTLSPlaintext(fragment CompletedTLSFragment) TLSPlaintextEvent {
	capturedLen := len(fragment.Payload)
	originalLen := int(fragment.OriginalLen)
	if originalLen == 0 {
		originalLen = int(fragment.TotalLen)
	}
	event := TLSPlaintextEvent{
		Type:         "raw",
		Timestamp:    time.Unix(0, int64(fragment.TimestampNS)).UTC(),
		PID:          fragment.PID,
		TGID:         fragment.TGID,
		Comm:         fragment.Comm,
		Direction:    tlsDirectionLabel(fragment.Direction),
		Lib:          tlsLibLabel(fragment.LibType),
		Function:     tlsFunctionLabel(fragment.Function),
		CapturedLen:  capturedLen,
		OriginalLen:  originalLen,
		Truncated:    fragment.Flags&tlsFlagTruncated != 0 || originalLen > capturedLen,
		RawAvailable: false,
		IsHandshake:  isTLSHandshakeFragment(fragment),
		UID:          readProcessUID(fragment.PID),
		TID:          fragment.PID,
	}

	if req, ok := parseTLSPlaintextHTTPRequest(fragment.Payload); ok {
		deps.CollectorMetrics.RecordAgentSightCounter("tls.http_request.parsed")
		return buildTLSPlaintextHTTPRequestEvent(event, req)
	}
	if resp, ok := parseTLSPlaintextHTTPResponse(fragment.Payload); ok {
		deps.CollectorMetrics.RecordAgentSightCounter("tls.http_response.parsed")
		return buildTLSPlaintextHTTPResponseEvent(event, resp)
	}

	event.RawHexDump = hexDump(fragment.Payload)
	deps.CollectorMetrics.RecordAgentSightCounter("tls.raw")
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
	base.URL = sanitizeTLSURL(parsed.req.URL.String())
	base.Host = parsed.host
	base.Headers = sanitizeTLSHeaders(parsed.req.Header)
	base.ContentType = parsed.content
	base.BodySize = parsed.bodySize
	base.Body, base.Truncated = formatTLSPlaintextBody(parsed.body, base.ContentType)
	base.Body = sanitizeTLSBody(base.Body, base.ContentType)
	base.RedactionState = "sanitized"
	annotateTLSSSEEvent(&base)
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
	base.Body = sanitizeTLSBody(base.Body, base.ContentType)
	base.RedactionState = "sanitized"
	annotateTLSSSEEvent(&base)
	base.RawAvailable = true
	return base
}

func sanitizeTLSHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	redacted := map[string]string{
		"authorization":       tlsRedactedValue,
		"proxy-authorization": tlsRedactedValue,
		"x-api-key":           tlsRedactedValue,
		"cookie":              tlsRedactedValue,
		"set-cookie":          tlsRedactedValue,
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

func sanitizeTLSURL(rawURL string) string {
	if strings.TrimSpace(rawURL) == "" {
		return rawURL
	}

	// First remove any embedded sensitive data (like API keys in URL)
	rawURL = RemoveSensitiveStringFromTLS(rawURL)

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return sanitizeTLSInlineSecrets(rawURL)
	}
	query := parsed.Query()
	changed := false
	for key := range query {
		if isTLSSensitiveKey(key) {
			query.Set(key, tlsRedactedValue)
			changed = true
		}
	}
	if !changed {
		return sanitizeTLSInlineSecrets(rawURL)
	}
	deps.CollectorMetrics.RecordAgentSightCounter("tls.redaction.url")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func sanitizeTLSBody(body, contentType string) string {
	if strings.TrimSpace(body) == "" {
		return body
	}

	// FIRST: Check if it is JSON and sanitize it structural-wise to avoid raw regex corrupting quotes
	if looksLikeTLSJSON(contentType, []byte(body)) {
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err == nil {
			// sanitizeTLSJSONValue will also run RemoveSensitiveStringFromTLS on every string value
			sanitizeTLSJSONValue(&payload)
			deps.CollectorMetrics.RecordAgentSightCounter("tls.redaction.body")
			if redacted, err := json.MarshalIndent(payload, "", "  "); err == nil {
				return string(redacted)
			}
		}
	}

	// For form urlencoded data
	if strings.Contains(strings.ToLower(contentType), "x-www-form-urlencoded") {
		if values, err := url.ParseQuery(body); err == nil {
			changed := false
			for key := range values {
				if isTLSSensitiveKey(key) {
					values.Set(key, tlsRedactedValue)
					changed = true
				}
			}
			if changed {
				deps.CollectorMetrics.RecordAgentSightCounter("tls.redaction.body")
				encoded := values.Encode()
				return RemoveSensitiveStringFromTLS(encoded)
			}
		}
	}

	// Otherwise, fallback to raw text key removal and inline secret sanitization
	body = RemoveSensitiveStringFromTLS(body)
	return sanitizeTLSInlineSecrets(body)
}

func sanitizeTLSJSONValue(value *any) bool {
	switch typed := (*value).(type) {
	case map[string]any:
		changed := false
		for key, child := range typed {
			if isTLSSensitiveKey(key) {
				typed[key] = tlsRedactedValue
				changed = true
				continue
			}
			childValue := child
			if sanitizeTLSJSONValue(&childValue) {
				typed[key] = childValue
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for i, child := range typed {
			childValue := child
			if sanitizeTLSJSONValue(&childValue) {
				typed[i] = childValue
				changed = true
			}
		}
		return changed
	case string:
		redacted := sanitizeTLSInlineSecrets(typed)
		if redacted != typed {
			*value = redacted
			return true
		}
	}
	return false
}

func isTLSSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if _, ok := tlsSensitiveQueryKeys[normalized]; ok {
		return true
	}
	return strings.Contains(normalized, "token") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "password") || strings.Contains(normalized, "api_key")
}

func sanitizeTLSInlineSecrets(value string) string {
	// First apply our comprehensive key removal (PEM keys, SSH keys, JWT, AWS, etc.)
	value = RemoveSensitiveStringFromTLS(value)

	// Then apply the existing inline secret patterns
	encodedRedactedValue := url.QueryEscape(tlsRedactedValue)
	placeholder := "__TLS_REDACTED_VALUE__"
	protected := strings.ReplaceAll(value, encodedRedactedValue, placeholder)
	redacted := tlsBearerTokenPattern.ReplaceAllString(protected, `${1}`+tlsRedactedValue)
	redacted = tlsInlineSecretPattern.ReplaceAllString(redacted, `${1}=`+tlsRedactedValue)
	redacted = strings.ReplaceAll(redacted, placeholder, encodedRedactedValue)
	if redacted != value {
		deps.CollectorMetrics.RecordAgentSightCounter("tls.redaction.inline")
	}
	return redacted
}

func annotateTLSSSEEvent(event *TLSPlaintextEvent) {
	if event == nil || !strings.Contains(strings.ToLower(event.ContentType), "text/event-stream") {
		return
	}
	event.Type = "sse_message"
	deps.CollectorMetrics.RecordAgentSightCounter("tls.sse.parsed")
	var dataParts []string
	for _, line := range strings.Split(event.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "event:") && event.SSEEvent == "" {
			event.SSEEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		}
		if strings.HasPrefix(trimmed, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if data != "" {
				dataParts = append(dataParts, data)
			}
		}
	}
	if len(dataParts) > 0 {
		event.SSEDataCount = len(dataParts)
		sum := sha256.Sum256([]byte(strings.Join(dataParts, "\n")))
		event.SSEDataDigest = "sha256:" + hex.EncodeToString(sum[:8])
	}
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
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
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

func tlsFunctionLabel(function uint8) string {
	switch function {
	case tlsFuncSSLWrite:
		return "SSL_write"
	case tlsFuncSSLRead:
		return "SSL_read"
	case tlsFuncSSLWriteEx:
		return "SSL_write_ex"
	case tlsFuncSSLReadEx:
		return "SSL_read_ex"
	case tlsFuncGnuTLSRecordSend:
		return "gnutls_record_send"
	case tlsFuncGnuTLSRecordRecv:
		return "gnutls_record_recv"
	case tlsFuncPRWrite:
		return "PR_Write"
	case tlsFuncPRRead:
		return "PR_Read"
	case tlsFuncGoConnWrite:
		return "crypto/tls.(*Conn).Write"
	case tlsFuncGoConnRead:
		return "crypto/tls.(*Conn).Read"
	default:
		return ""
	}
}
