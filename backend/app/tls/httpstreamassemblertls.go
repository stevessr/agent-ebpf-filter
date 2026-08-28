package tls

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section httpstreamassemblertls.go ----

const tlsHTTPStreamMaxBuffer = 512 * 1024
const tlsHTTPStreamRequestQueueLimit = 64

type tlsHTTPStreamKey struct {
	PID          uint32
	TGID         uint32
	ConnectionID uint64
	LibType      uint8
}

type tlsHTTPBufferKey struct {
	PID          uint32
	TGID         uint32
	ConnectionID uint64
	LibType      uint8
	Direction    uint8
}

type pendingTLSHTTPStream struct {
	firstSeen time.Time
	lastSeen  time.Time
	meta      CompletedTLSFragment
	buffer    []byte
	flags     uint8
}

type tlsHTTPRequestContext struct {
	Method string
	URL    string
	Host   string
}

type TLSHTTPStreamAssembler struct {
	mu       sync.Mutex
	pending  map[tlsHTTPBufferKey]*pendingTLSHTTPStream
	requests map[tlsHTTPStreamKey][]tlsHTTPRequestContext
	timeout  time.Duration
	dropped  int
}

func NewTLSHTTPStreamAssembler(timeout time.Duration) *TLSHTTPStreamAssembler {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &TLSHTTPStreamAssembler{
		pending:  make(map[tlsHTTPBufferKey]*pendingTLSHTTPStream),
		requests: make(map[tlsHTTPStreamKey][]tlsHTTPRequestContext),
		timeout:  timeout,
	}
}

func (a *TLSHTTPStreamAssembler) Add(fragment CompletedTLSFragment) []TLSPlaintextEvent {
	if a == nil || len(fragment.Payload) == 0 {
		return nil
	}
	var now time.Time
	if fragment.TimestampNS == 0 {
		now = time.Now()
	} else {
		// TimestampNS is monotonic since boot. Using Unix epoch as an arbitrary
		// origin is intentional here: stream expiry only depends on deltas.
		now = time.Unix(0, int64(fragment.TimestampNS))
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.cleanupExpiredLocked(now)

	key := tlsHTTPBufferKey{
		PID:          fragment.PID,
		TGID:         fragment.TGID,
		ConnectionID: fragment.ConnectionID,
		LibType:      fragment.LibType,
		Direction:    fragment.Direction,
	}
	pending := a.pending[key]
	if pending == nil {
		payload := trimTLSHTTPMessageSeparators(fragment.Payload)
		if !looksLikeTLSHTTPMessageStart(payload, fragment.Direction) {
			return nil
		}
		meta := fragment
		meta.Payload = nil
		pending = &pendingTLSHTTPStream{
			firstSeen: now,
			lastSeen:  now,
			meta:      meta,
			buffer:    append([]byte(nil), payload...),
			flags:     fragment.Flags,
		}
		a.pending[key] = pending
	} else {
		pending.lastSeen = now
		pending.flags |= fragment.Flags
		pending.buffer = append(pending.buffer, fragment.Payload...)
		if len(pending.buffer) > tlsHTTPStreamMaxBuffer {
			delete(a.pending, key)
			a.dropped++
			return nil
		}
	}

	var events []TLSPlaintextEvent
	for {
		pending.buffer = trimTLSHTTPMessageSeparators(pending.buffer)
		if len(pending.buffer) == 0 {
			delete(a.pending, key)
			return events
		}
		messageLen, complete, invalid := tlsCompleteHTTPMessageLength(pending.buffer, fragment.Direction)
		if invalid {
			delete(a.pending, key)
			a.dropped++
			return events
		}
		if !complete {
			return events
		}

		messagePayload := append([]byte(nil), pending.buffer[:messageLen]...)
		messageFragment := pending.meta
		messageFragment.TimestampNS = fragment.TimestampNS
		messageFragment.Payload = messagePayload
		messageFragment.TotalLen = uint32(len(messagePayload))
		messageFragment.OriginalLen = uint32(len(messagePayload))
		messageFragment.DataLen = uint32(len(messagePayload))
		messageFragment.FragCount = 1
		messageFragment.Flags = pending.flags
		event := parseTLSPlaintext(messageFragment)
		if isTLSHTTPDisplayEvent(event) {
			a.trackHTTPEventLocked(key, &event)
			events = append(events, event)
		}

		pending.buffer = pending.buffer[messageLen:]
		pending.flags = 0
	}
}

func (a *TLSHTTPStreamAssembler) Pending() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pending)
}

func (a *TLSHTTPStreamAssembler) Dropped() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dropped
}

func (a *TLSHTTPStreamAssembler) cleanupExpiredLocked(now time.Time) {
	for key, pending := range a.pending {
		if now.Sub(pending.lastSeen) > a.timeout || now.Sub(pending.firstSeen) > 2*a.timeout {
			delete(a.pending, key)
			a.dropped++
		}
	}
}

func (a *TLSHTTPStreamAssembler) trackHTTPEventLocked(key tlsHTTPBufferKey, event *TLSPlaintextEvent) {
	streamKey := tlsHTTPStreamKey{
		PID:          key.PID,
		TGID:         key.TGID,
		ConnectionID: key.ConnectionID,
		LibType:      key.LibType,
	}
	if event.Type == "http_request" {
		queue := append(a.requests[streamKey], tlsHTTPRequestContext{Method: event.Method, URL: event.URL, Host: event.Host})
		if len(queue) > tlsHTTPStreamRequestQueueLimit {
			queue = queue[len(queue)-tlsHTTPStreamRequestQueueLimit:]
		}
		a.requests[streamKey] = queue
		return
	}
	if event.Type != "http_response" && event.Type != "sse_message" {
		return
	}
	queue := a.requests[streamKey]
	if len(queue) == 0 {
		return
	}
	request := queue[0]
	if len(queue) == 1 {
		delete(a.requests, streamKey)
	} else {
		a.requests[streamKey] = queue[1:]
	}
	if event.Method == "" {
		event.Method = request.Method
	}
	if event.URL == "" {
		event.URL = request.URL
	}
	if event.Host == "" {
		event.Host = request.Host
	}
}

func isTLSHTTPDisplayEvent(event TLSPlaintextEvent) bool {
	return event.Type == "http_request" || event.Type == "http_response" || event.Type == "sse_message"
}

func trimTLSHTTPMessageSeparators(payload []byte) []byte {
	return bytes.TrimLeft(payload, "\r\n")
}

func looksLikeTLSHTTPMessageStart(payload []byte, direction uint8) bool {
	if len(payload) == 0 {
		return false
	}
	if direction == tlsDirectionRecv {
		return bytes.HasPrefix(payload, []byte("HTTP/"))
	}
	return looksLikeTLSHTTPRequest(payload)
}

func tlsCompleteHTTPMessageLength(payload []byte, direction uint8) (int, bool, bool) {
	if !looksLikeTLSHTTPMessageStart(payload, direction) {
		return 0, false, true
	}
	headerEnd, separatorLen := tlsHTTPHeaderEnd(payload)
	if headerEnd < 0 {
		return 0, false, false
	}
	headersStart := tlsHTTPFirstLineEnd(payload)
	if headersStart < 0 || headersStart > headerEnd {
		return 0, false, true
	}
	headers := parseTLSHTTPHeaderMap(string(payload[headersStart:headerEnd]))
	bodyStart := headerEnd + separatorLen

	if tlsHTTPHasChunkedTransfer(headers) {
		bodyLen, ok := tlsChunkedBodyLength(payload[bodyStart:])
		if !ok {
			return 0, false, false
		}
		return bodyStart + bodyLen, true, false
	}

	if contentLength, ok, valid := tlsHTTPContentLength(headers); ok || !valid {
		if !valid || contentLength < 0 {
			return 0, false, true
		}
		if len(payload)-bodyStart < contentLength {
			return 0, false, false
		}
		return bodyStart + contentLength, true, false
	}

	if direction == tlsDirectionRecv && tlsHTTPResponseHasNoBody(payload) {
		return bodyStart, true, false
	}
	if direction == tlsDirectionSend {
		return bodyStart, true, false
	}
	return 0, false, false
}

func tlsHTTPHeaderEnd(payload []byte) (int, int) {
	if idx := bytes.Index(payload, []byte("\r\n\r\n")); idx >= 0 {
		return idx, 4
	}
	if idx := bytes.Index(payload, []byte("\n\n")); idx >= 0 {
		return idx, 2
	}
	return -1, 0
}

func tlsHTTPFirstLineEnd(payload []byte) int {
	if idx := bytes.Index(payload, []byte("\r\n")); idx >= 0 {
		return idx + 2
	}
	if idx := bytes.IndexByte(payload, '\n'); idx >= 0 {
		return idx + 1
	}
	return -1
}

func parseTLSHTTPHeaderMap(raw string) map[string][]string {
	headers := make(map[string][]string)
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		headers[key] = append(headers[key], value)
	}
	return headers
}

func tlsHTTPHasChunkedTransfer(headers map[string][]string) bool {
	for _, value := range headers["transfer-encoding"] {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "chunked") {
				return true
			}
		}
	}
	return false
}

func tlsHTTPContentLength(headers map[string][]string) (int, bool, bool) {
	values := headers["content-length"]
	if len(values) == 0 {
		return 0, false, true
	}
	value := strings.TrimSpace(values[len(values)-1])
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, true, false
	}
	return parsed, true, true
}

func tlsHTTPResponseHasNoBody(payload []byte) bool {
	lineEnd := bytes.IndexAny(payload, "\r\n")
	if lineEnd <= 0 {
		return false
	}
	parts := strings.Fields(string(payload[:lineEnd]))
	if len(parts) < 2 {
		return false
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return (status >= 100 && status < 200) || status == 204 || status == 304
}

func tlsChunkedBodyLength(body []byte) (int, bool) {
	offset := 0
	for {
		lineEnd, lineBreakLen := tlsChunkLineEnd(body[offset:])
		if lineEnd < 0 {
			return 0, false
		}
		sizeLine := string(body[offset : offset+lineEnd])
		if semi := strings.IndexByte(sizeLine, ';'); semi >= 0 {
			sizeLine = sizeLine[:semi]
		}
		size, err := strconv.ParseInt(strings.TrimSpace(sizeLine), 16, 64)
		if err != nil || size < 0 {
			return 0, false
		}
		offset += lineEnd + lineBreakLen
		if size == 0 {
			trailerLen, ok := tlsChunkTrailerLength(body[offset:])
			if !ok {
				return 0, false
			}
			return offset + trailerLen, true
		}
		need := int(size) + 2
		if len(body)-offset < need {
			return 0, false
		}
		if !bytes.Equal(body[offset+int(size):offset+need], []byte("\r\n")) {
			return 0, false
		}
		offset += need
	}
}

func tlsChunkLineEnd(body []byte) (int, int) {
	if idx := bytes.Index(body, []byte("\r\n")); idx >= 0 {
		return idx, 2
	}
	if idx := bytes.IndexByte(body, '\n'); idx >= 0 {
		return idx, 1
	}
	return -1, 0
}

func tlsChunkTrailerLength(body []byte) (int, bool) {
	if len(body) >= 2 && bytes.Equal(body[:2], []byte("\r\n")) {
		return 2, true
	}
	if len(body) >= 1 && body[0] == '\n' {
		return 1, true
	}
	if idx := bytes.Index(body, []byte("\r\n\r\n")); idx >= 0 {
		return idx + 4, true
	}
	if idx := bytes.Index(body, []byte("\n\n")); idx >= 0 {
		return idx + 2, true
	}
	return 0, false
}
