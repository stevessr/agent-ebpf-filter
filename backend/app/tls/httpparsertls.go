package tls

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"strings"
)

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
	body, bodyTruncated := formatEncodedTLSPlaintextBody(parsed.body, base.ContentType, parsed.req.Header.Get("Content-Encoding"))
	base.Body = body
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
	body, bodyTruncated := formatEncodedTLSPlaintextBody(parsed.body, base.ContentType, parsed.resp.Header.Get("Content-Encoding"))
	base.Body = body
	base.Truncated = base.Truncated || bodyTruncated || declaredTLSBodyIncomplete(parsed.resp.ContentLength, len(parsed.body))
	base.RedactionState = "sanitized"
	annotateTLSSSEEvent(&base)
	base.RawAvailable = true
	return base
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
