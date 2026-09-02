package tls

import (
	"bytes"
	"time"
)

const tlsHTTPMaxHeaderBytes = 64 * 1024
const tlsHTTPMaxMessagesPerCapture = 64

var tlsHTTPRequestPrefixes = [][]byte{
	[]byte("GET "),
	[]byte("POST "),
	[]byte("PUT "),
	[]byte("PATCH "),
	[]byte("DELETE "),
	[]byte("HEAD "),
	[]byte("OPTIONS "),
	[]byte("TRACE "),
	[]byte("CONNECT "),
}

func tlsHTTPBufferKeyForFragment(fragment CompletedTLSFragment) tlsHTTPBufferKey {
	return tlsHTTPBufferKey{
		PID:          fragment.PID,
		TGID:         fragment.TGID,
		ConnectionID: fragment.ConnectionID,
		LibType:      fragment.LibType,
		Direction:    fragment.Direction,
	}
}

func tlsHTTPStreamTimestamp(fragment CompletedTLSFragment) time.Time {
	if fragment.TimestampNS == 0 {
		return time.Now()
	}
	// As in the core assembler, the monotonic value is used only as a stable
	// origin for relative expiry calculations.
	return time.Unix(0, int64(fragment.TimestampNS))
}

// looksLikeTLSHTTPMessagePrefix recognizes a first line that was itself split
// across TLS capture events. Prefer false-positive suppression of raw output
// over leaking the remainder of an Authorization/body-bearing request: an
// invalid continuation will be discarded by the normal HTTP parser.
func looksLikeTLSHTTPMessagePrefix(payload []byte, direction uint8) bool {
	payload = trimTLSHTTPMessageSeparators(payload)
	if len(payload) == 0 {
		return false
	}
	if direction == tlsDirectionRecv {
		marker := []byte("HTTP/")
		return bytes.HasPrefix(marker, payload) || bytes.HasPrefix(payload, marker)
	}
	for _, method := range tlsHTTPRequestPrefixes {
		if bytes.HasPrefix(method, payload) || bytes.HasPrefix(payload, method) {
			return true
		}
	}
	return false
}

// seedTLSHTTPPrefixLocked stores an HTTP-looking first-line prefix that the
// original assembler cannot classify until a line terminator arrives.
func (a *TLSHTTPStreamAssembler) seedTLSHTTPPrefixLocked(fragment CompletedTLSFragment, key tlsHTTPBufferKey) bool {
	payload := trimTLSHTTPMessageSeparators(fragment.Payload)
	if !looksLikeTLSHTTPMessagePrefix(payload, fragment.Direction) {
		return false
	}
	now := tlsHTTPStreamTimestamp(fragment)
	a.cleanupExpiredLocked(now)
	meta := fragment
	meta.Payload = nil
	a.pending[key] = &pendingTLSHTTPStream{
		firstSeen: now,
		lastSeen:  now,
		meta:      meta,
		buffer:    append([]byte(nil), payload...),
		flags:     fragment.Flags,
	}
	return true
}

func boundedTLSHTTPHeaderBlock(payload []byte) ([]byte, bool, string) {
	headerEnd, _ := tlsHTTPHeaderEnd(payload)
	if headerEnd < 0 {
		if len(payload) > tlsHTTPMaxHeaderBytes {
			return nil, false, "HTTP/1 header exceeds bounded parser limit"
		}
		return nil, false, ""
	}
	if headerEnd > tlsHTTPMaxHeaderBytes {
		return nil, true, "HTTP/1 header exceeds bounded parser limit"
	}
	firstLineEnd := tlsHTTPFirstLineEnd(payload)
	if firstLineEnd < 0 || firstLineEnd > headerEnd {
		return nil, true, "invalid HTTP/1 start line"
	}
	return payload[firstLineEnd:headerEnd], true, ""
}

func parseBoundedTLSHTTPContentLength(token []byte) (value int64, valid, exceedsLimit bool) {
	token = bytes.TrimSpace(token)
	if len(token) == 0 {
		return 0, false, false
	}
	limit := int64(tlsHTTPStreamMaxBuffer)
	for _, rawDigit := range token {
		if rawDigit < '0' || rawDigit > '9' {
			return 0, false, false
		}
		if exceedsLimit {
			continue
		}
		digit := int64(rawDigit - '0')
		if value > (limit-digit)/10 {
			exceedsLimit = true
			continue
		}
		value = value*10 + digit
	}
	return value, true, exceedsLimit
}

// validateTLSHTTPHeaderFraming rejects request-smuggling-style ambiguity before
// the permissive display parser sees it. We deliberately fail closed because
// Agent Sight should never guess which bytes belong to a sensitive request.
func validateTLSHTTPHeaderFraming(payload []byte) (headerComplete bool, reason string) {
	headerBlock, complete, reason := boundedTLSHTTPHeaderBlock(payload)
	if reason != "" || !complete {
		return complete, reason
	}

	var contentLength int64
	contentLengthSeen := false
	transferCodingCount := 0
	transferCodingIsChunked := true
	for len(headerBlock) > 0 {
		lineEnd := bytes.IndexByte(headerBlock, '\n')
		line := headerBlock
		if lineEnd >= 0 {
			line = headerBlock[:lineEnd]
			headerBlock = headerBlock[lineEnd+1:]
		} else {
			headerBlock = nil
		}
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			return true, "obsolete folded HTTP/1 header is not accepted"
		}
		separator := bytes.IndexByte(line, ':')
		if separator <= 0 {
			return true, "malformed HTTP/1 header line"
		}
		rawName := line[:separator]
		if !bytes.Equal(rawName, bytes.TrimSpace(rawName)) {
			return true, "whitespace around HTTP/1 header name is not accepted"
		}
		value := bytes.TrimSpace(line[separator+1:])
		switch {
		case bytes.EqualFold(rawName, []byte("content-length")):
			for {
				tokenEnd := bytes.IndexByte(value, ',')
				token := value
				if tokenEnd >= 0 {
					token = value[:tokenEnd]
					value = value[tokenEnd+1:]
				} else {
					value = nil
				}
				parsed, valid, exceedsLimit := parseBoundedTLSHTTPContentLength(token)
				if !valid {
					return true, "invalid Content-Length value"
				}
				if exceedsLimit {
					return true, "Content-Length exceeds HTTP/1 stream buffer limit"
				}
				if contentLengthSeen && contentLength != parsed {
					return true, "conflicting Content-Length values"
				}
				contentLength = parsed
				contentLengthSeen = true
				if tokenEnd < 0 {
					break
				}
			}
		case bytes.EqualFold(rawName, []byte("transfer-encoding")):
			for {
				tokenEnd := bytes.IndexByte(value, ',')
				token := value
				if tokenEnd >= 0 {
					token = value[:tokenEnd]
					value = value[tokenEnd+1:]
				} else {
					value = nil
				}
				if parameter := bytes.IndexByte(token, ';'); parameter >= 0 {
					token = token[:parameter]
				}
				coding := bytes.TrimSpace(token)
				if len(coding) == 0 {
					return true, "invalid Transfer-Encoding value"
				}
				transferCodingCount++
				transferCodingIsChunked = transferCodingIsChunked && bytes.EqualFold(coding, []byte("chunked"))
				if tokenEnd < 0 {
					break
				}
			}
		}
	}

	if contentLengthSeen && transferCodingCount > 0 {
		return true, "Transfer-Encoding with Content-Length is ambiguous"
	}
	if transferCodingCount > 0 {
		// Go's bounded display parser only decodes a single chunked transfer
		// coding. Accepting a syntactically valid chain such as gzip, chunked
		// would consume the stream without producing an event, hiding the drop
		// from capture diagnostics. Reject unsupported chains explicitly.
		if transferCodingCount != 1 || !transferCodingIsChunked {
			return true, "unsupported or ambiguous Transfer-Encoding chain"
		}
	}
	return true, ""
}

// validateTLSHTTPFramingSequence checks every complete pipelined message that
// is already present. It stops at an incomplete body/header; later capture
// fragments will resume validation before parsing continues.
func validateTLSHTTPFramingSequence(payload []byte, direction uint8) string {
	remaining := trimTLSHTTPMessageSeparators(payload)
	for messages := 0; messages < tlsHTTPMaxMessagesPerCapture && len(remaining) > 0; messages++ {
		if !looksLikeTLSHTTPMessagePrefix(remaining, direction) {
			return ""
		}
		headerComplete, reason := validateTLSHTTPHeaderFraming(remaining)
		if reason != "" {
			return reason
		}
		if !headerComplete {
			return ""
		}
		messageLen, complete, invalid := tlsCompleteHTTPMessageLength(remaining, direction)
		if invalid {
			return "invalid HTTP/1 message framing"
		}
		if !complete {
			return ""
		}
		if messageLen <= 0 || messageLen > len(remaining) {
			return "invalid HTTP/1 message length"
		}
		remaining = trimTLSHTTPMessageSeparators(remaining[messageLen:])
	}
	if len(remaining) > 0 {
		return "too many pipelined HTTP/1 messages in one capture"
	}
	return ""
}

func tlsHTTPFramingCandidateLocked(a *TLSHTTPStreamAssembler, key tlsHTTPBufferKey, fragment CompletedTLSFragment) []byte {
	pending := a.pending[key]
	if pending == nil || len(pending.buffer) == 0 {
		return trimTLSHTTPMessageSeparators(fragment.Payload)
	}
	// Only copy the bounded HTTP assembler state. This is a security slow-path
	// for a partially assembled message, not the common one-shot request path.
	candidate := make([]byte, 0, len(pending.buffer)+len(fragment.Payload))
	candidate = append(candidate, pending.buffer...)
	candidate = append(candidate, fragment.Payload...)
	return trimTLSHTTPMessageSeparators(candidate)
}

// AddRecognized mirrors Add while additionally reporting ownership. The read
// loop uses this signal to suppress raw TLS fallback for partial HTTP/1 bytes.
// A probe-level truncation is a known capture gap: any residual pending stream
// is removed after parsing the captured prefix so later SSL calls can never be
// stitched across missing bytes into a fabricated request/response.
func (a *TLSHTTPStreamAssembler) AddRecognized(fragment CompletedTLSFragment) ([]TLSPlaintextEvent, bool) {
	if a == nil || len(fragment.Payload) == 0 {
		return nil, false
	}
	key := tlsHTTPBufferKeyForFragment(fragment)
	payload := trimTLSHTTPMessageSeparators(fragment.Payload)

	a.mu.Lock()
	_, alreadyOwned := a.pending[key]
	recognized := alreadyOwned || looksLikeTLSHTTPMessageStart(payload, fragment.Direction) || looksLikeTLSHTTPMessagePrefix(payload, fragment.Direction)
	if recognized {
		candidate := tlsHTTPFramingCandidateLocked(a, key, fragment)
		if reason := validateTLSHTTPFramingSequence(candidate, fragment.Direction); reason != "" {
			if alreadyOwned {
				delete(a.pending, key)
			}
			a.dropped++
			a.mu.Unlock()
			return nil, true
		}
	}
	if !alreadyOwned && !looksLikeTLSHTTPMessageStart(payload, fragment.Direction) {
		if a.seedTLSHTTPPrefixLocked(fragment, key) {
			if fragment.Flags&tlsFlagTruncated != 0 {
				delete(a.pending, key)
				a.dropped++
			}
			a.mu.Unlock()
			return nil, true
		}
	}
	a.mu.Unlock()

	events := a.Add(fragment)
	if !recognized {
		return events, false
	}

	if fragment.Flags&tlsFlagTruncated != 0 {
		a.mu.Lock()
		if _, pending := a.pending[key]; pending {
			delete(a.pending, key)
			a.dropped++
		}
		a.mu.Unlock()
	}
	return events, true
}
