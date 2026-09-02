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
	"unicode/utf8"
)

// Keep the formatted HTTP body below the expanded eBPF plaintext capture
// window (35,712 bytes) so headers and framing still have headroom. The old
// 16 KiB limit discarded useful prompt/tool payload even when the kernel had
// already captured it successfully.
const tlsMaxBodySize = 32 * 1024

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
		Timestamp:    bpfKtimeToWallClock(fragment.TimestampNS),
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

func declaredTLSBodyIncomplete(contentLength int64, captured int) bool {
	return contentLength >= 0 && contentLength > int64(captured)
}

func buildTLSPlaintextHTTPRequestEvent(base TLSPlaintextEvent, parsed *tlsHTTPRequest) TLSPlaintextEvent {
	base.Type = "http_request"
	base.Method = parsed.req.Method
	base.URL = sanitizeTLSURL(parsed.req.URL.String())
	base.Host = parsed.host
	base.Headers = sanitizeTLSHeaders(parsed.req.Header)
	base.ContentType = parsed.content
	base.BodySize = parsed.bodySize
	body, bodyTruncated := formatTLSPlaintextBody(parsed.body, base.ContentType)
	base.Body = sanitizeTLSBody(body, base.ContentType)
	// Never erase the eBPF capture-level truncation flag. The previous tuple
	// assignment overwrote it with the display-body result, making truncated
	// kernel captures appear complete in Agent Sight.
	base.Truncated = base.Truncated || bodyTruncated || declaredTLSBodyIncomplete(parsed.req.ContentLength, len(parsed.body))
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
	body, bodyTruncated := formatTLSPlaintextBody(parsed.body, base.ContentType)
	base.Body = sanitizeTLSBody(body, base.ContentType)
	base.Truncated = base.Truncated || bodyTruncated || declaredTLSBodyIncomplete(parsed.resp.ContentLength, len(parsed.body))
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

func truncateTLSBodyUTF8(body []byte, limit int) []byte {
	if len(body) <= limit {
		return body
	}
	if !utf8.Valid(body) {
		body = bytes.ToValidUTF8(body, []byte("\uFFFD"))
		if len(body) <= limit {
			return body
		}
	}
	end := limit
	for end > 0 && !utf8.Valid(body[:end]) {
		end--
	}
	return body[:end]
}

func formatTLSPlaintextBody(body []byte, contentType string) (string, bool) {
	if len(body) == 0 {
		return "", false
	}

	// Capture whether the original HTTP body exceeded the bounded window
	// BEFORE folding — folding shrinks the body, so checking afterwards would
	// mask the fact that the capture was truncated.
	truncated := len(body) > tlsMaxBodySize

	// Fold large base64 image payloads (Anthropic `image` blocks and OpenAI
	// `image_url` data URIs) into compact sentinels BEFORE pretty-print. A
	// single embedded image would otherwise consume most of the 32 KiB display
	// window. Folding also keeps base64 substrings from spuriously triggering
	// inline-secret redaction.
	folded := foldTLSImageBase64(body, contentType)

	formatted := folded
	if looksLikeTLSJSON(contentType, folded) {
		if pretty, err := prettyPrintJSON(folded); err == nil {
			formatted = pretty
		}
	}
	if len(formatted) > tlsMaxBodySize {
		truncated = true
		formatted = truncateTLSBodyUTF8(formatted, tlsMaxBodySize)
	}
	return string(formatted), truncated
}

// tlsImageFoldThreshold is the base64 length below which an embedded image is
// left intact so the frontend can still render it as a data URI. Anything
// larger is folded to a sentinel to preserve prompt/tool text inside the
// bounded 35 KiB plaintext capture window.
const tlsImageFoldThreshold = 2048

// tlsImageFoldedPrefix marks a folded image payload. The full sentinel format
// is `__IMAGE_FOLDED__:<media_type>:<approx_decoded_bytes>`; the frontend
// detects this prefix to render an image placeholder instead of raw base64.
const tlsImageFoldedPrefix = "__IMAGE_FOLDED__:"

// foldTLSImageBase64 walks a JSON body and replaces large base64 image
// payloads with a compact sentinel. Returns the original body unchanged if it
// is not JSON or contains no foldable images.
func foldTLSImageBase64(body []byte, contentType string) []byte {
	if !looksLikeTLSJSON(contentType, body) {
		return body
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	if !foldTLSImageJSONValue(&payload) {
		return body
	}
	if folded, err := json.Marshal(payload); err == nil {
		return folded
	}
	return body
}

// foldTLSImageJSONValue recursively folds large image payloads in a parsed
// JSON value. It recognises:
//   - Anthropic: {"type":"image","source":{"type":"base64","media_type":"...","data":"..."}}
//   - OpenAI:    {"type":"image_url","image_url":{"url":"data:image/png;base64,..."}}
//
// HTTP(S) image URLs are left intact (they are small and useful to display).
func foldTLSImageJSONValue(value *any) bool {
	switch typed := (*value).(type) {
	case map[string]any:
		changed := false
		switch t, _ := typed["type"].(string); t {
		case "image":
			if src, ok := typed["source"].(map[string]any); ok {
				if st, _ := src["type"].(string); st == "base64" {
					if data, ok := src["data"].(string); ok && len(data) > tlsImageFoldThreshold {
						media, _ := src["media_type"].(string)
						src["data"] = tlsImageFoldedSentinel(media, len(data))
						changed = true
					}
				}
			}
		case "image_url":
			if iu, ok := typed["image_url"].(map[string]any); ok {
				if rawURL, ok := iu["url"].(string); ok {
					if folded, ok := foldTLSDataURI(rawURL); ok {
						iu["url"] = folded
						changed = true
					}
				}
			}
		}
		for key, child := range typed {
			childValue := child
			if foldTLSImageJSONValue(&childValue) {
				typed[key] = childValue
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for i, child := range typed {
			childValue := child
			if foldTLSImageJSONValue(&childValue) {
				typed[i] = childValue
				changed = true
			}
		}
		return changed
	}
	return false
}

// foldTLSDataURI folds the base64 portion of a `data:<media>;base64,<...>` URI
// when it exceeds the threshold. Non-data URIs (http(s) URLs) are returned
// unchanged with ok=false.
func foldTLSDataURI(raw string) (string, bool) {
	const prefix = "data:"
	if !strings.HasPrefix(raw, prefix) {
		return raw, false
	}
	comma := strings.Index(raw, ",")
	if comma < 0 {
		return raw, false
	}
	meta := raw[len(prefix):comma] // e.g. "image/png;base64"
	data := raw[comma+1:]
	if !strings.Contains(meta, "base64") || len(data) <= tlsImageFoldThreshold {
		return raw, false
	}
	media := strings.SplitN(meta, ";", 2)[0]
	return tlsImageFoldedSentinel(media, len(data)), true
}

// tlsImageFoldedSentinel builds a compact placeholder for a folded image.
// approxBytes is the base64 length; the decoded byte size is ~3/4 of that.
func tlsImageFoldedSentinel(mediaType string, base64Len int) string {
	approx := base64Len * 3 / 4
	if mediaType == "" {
		mediaType = "image"
	}
	return fmt.Sprintf("%s%s:%d", tlsImageFoldedPrefix, mediaType, approx)
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
	case tlsFuncRustlsEncryptOutgoing:
		return "rustls::encrypt_outgoing"
	case tlsFuncRustlsConsumeFirstChunk:
		return "rustls::consume_first_chunk"
	default:
		return ""
	}
}
