package tls

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	tlsHTTP2FrameHeaderSize = 9
	tlsHTTP2MaxFramePayload = 1 << 20
	tlsHTTP2MaxBuffer       = 2 << 20
)

var tlsHTTP2ClientPreface = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")

type tlsHTTP2StreamKey struct {
	PID          uint32
	TGID         uint32
	ConnectionID uint64
	LibType      uint8
	Direction    uint8
}

type pendingTLSHTTP2Stream struct {
	firstSeen time.Time
	lastSeen  time.Time
	meta      CompletedTLSFragment
	buffer    []byte
	flags     uint8
}

type TLSHTTP2StreamAssembler struct {
	mu      sync.Mutex
	pending map[tlsHTTP2StreamKey]*pendingTLSHTTP2Stream
	timeout time.Duration
	dropped int
}

func NewTLSHTTP2StreamAssembler(timeout time.Duration) *TLSHTTP2StreamAssembler {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &TLSHTTP2StreamAssembler{
		pending: make(map[tlsHTTP2StreamKey]*pendingTLSHTTP2Stream),
		timeout: timeout,
	}
}

func tlsHTTP2StreamKeyFor(fragment CompletedTLSFragment) tlsHTTP2StreamKey {
	return tlsHTTP2StreamKey{
		PID:          fragment.PID,
		TGID:         fragment.TGID,
		ConnectionID: fragment.ConnectionID,
		LibType:      fragment.LibType,
		Direction:    fragment.Direction,
	}
}

// Add returns complete HTTP/2 events and whether the payload belongs to an
// HTTP/2 stream. A recognized but incomplete frame returns (nil, true), which
// lets the read loop avoid publishing misleading raw fragments while waiting
// for the rest of the frame.
func (a *TLSHTTP2StreamAssembler) Add(fragment CompletedTLSFragment) ([]TLSPlaintextEvent, bool) {
	if a == nil || len(fragment.Payload) == 0 {
		return nil, false
	}

	now := time.Now()
	key := tlsHTTP2StreamKeyFor(fragment)

	a.mu.Lock()
	defer a.mu.Unlock()
	a.cleanupExpiredLocked(now)

	pending := a.pending[key]
	if pending == nil {
		if !looksLikeTLSHTTP2Start(fragment.Payload) {
			return nil, false
		}
		meta := fragment
		meta.Payload = nil
		pending = &pendingTLSHTTP2Stream{
			firstSeen: now,
			lastSeen:  now,
			meta:      meta,
			buffer:    make([]byte, 0, min(tlsHTTP2MaxBuffer, len(fragment.Payload)+tlsHTTP2FrameHeaderSize)),
		}
		a.pending[key] = pending
	}

	pending.lastSeen = now
	pending.meta.TimestampNS = fragment.TimestampNS
	pending.meta.Function = fragment.Function
	pending.flags |= fragment.Flags
	pending.buffer = append(pending.buffer, fragment.Payload...)
	if len(pending.buffer) > tlsHTTP2MaxBuffer {
		delete(a.pending, key)
		a.dropped++
		return []TLSPlaintextEvent{tlsHTTP2FallbackEvent(fragment, pending.buffer, true)}, true
	}

	events, valid := consumeTLSHTTP2Pending(pending)
	if !valid {
		delete(a.pending, key)
		a.dropped++
		return append(events, tlsHTTP2FallbackEvent(fragment, pending.buffer, false)), true
	}

	if fragment.Flags&tlsFlagTruncated != 0 && len(pending.buffer) > 0 {
		// The tail of this TLS call was not captured, so an incomplete HTTP/2
		// frame can never be reconstructed correctly. Drop the remainder now
		// instead of prepending it to the next TLS call on the connection.
		events = append(events, tlsHTTP2FallbackEvent(fragment, pending.buffer, true))
		pending.buffer = nil
		a.dropped++
	}

	if len(pending.buffer) == 0 {
		delete(a.pending, key)
	}
	return events, true
}

func (a *TLSHTTP2StreamAssembler) cleanupExpiredLocked(now time.Time) {
	for key, pending := range a.pending {
		if now.Sub(pending.lastSeen) > a.timeout || now.Sub(pending.firstSeen) > 2*a.timeout {
			delete(a.pending, key)
			a.dropped++
		}
	}
}

func (a *TLSHTTP2StreamAssembler) Pending() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pending)
}

func (a *TLSHTTP2StreamAssembler) Dropped() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dropped
}

func looksLikeTLSHTTP2Start(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if len(data) < len(tlsHTTP2ClientPreface) && bytes.Equal(data, tlsHTTP2ClientPreface[:len(data)]) {
		return true
	}
	if bytes.HasPrefix(data, tlsHTTP2ClientPreface) {
		return true
	}
	return len(data) >= tlsHTTP2FrameHeaderSize && validTLSHTTP2FrameHeader(data[:tlsHTTP2FrameHeaderSize])
}

func validTLSHTTP2FrameHeader(header []byte) bool {
	if len(header) < tlsHTTP2FrameHeaderSize || header[5]&0x80 != 0 {
		return false
	}
	length := tlsHTTP2FrameLength(header)
	if length < 0 || length > tlsHTTP2MaxFramePayload {
		return false
	}
	frameType := header[3]
	flags := header[4]
	streamID := tlsHTTP2StreamID(header)

	switch frameType {
	case 0x0: // DATA
		return streamID != 0
	case 0x1: // HEADERS
		return streamID != 0
	case 0x2: // PRIORITY
		return streamID != 0 && length == 5
	case 0x3: // RST_STREAM
		return streamID != 0 && length == 4
	case 0x4: // SETTINGS
		if streamID != 0 {
			return false
		}
		if flags&0x1 != 0 {
			return length == 0
		}
		return length%6 == 0
	case 0x5: // PUSH_PROMISE
		return streamID != 0 && length >= 4
	case 0x6: // PING
		return streamID == 0 && length == 8
	case 0x7: // GOAWAY
		return streamID == 0 && length >= 8
	case 0x8: // WINDOW_UPDATE
		return length == 4
	case 0x9: // CONTINUATION
		return streamID != 0
	default:
		return false
	}
}

func tlsHTTP2FrameLength(header []byte) int {
	if len(header) < 3 {
		return -1
	}
	return int(header[0])<<16 | int(header[1])<<8 | int(header[2])
}

func tlsHTTP2StreamID(header []byte) uint32 {
	if len(header) < tlsHTTP2FrameHeaderSize {
		return 0
	}
	return binary.BigEndian.Uint32(header[5:9]) & 0x7fffffff
}

func consumeTLSHTTP2Pending(pending *pendingTLSHTTP2Stream) ([]TLSPlaintextEvent, bool) {
	if pending == nil {
		return nil, false
	}
	var events []TLSPlaintextEvent

	if len(pending.buffer) < len(tlsHTTP2ClientPreface) && bytes.Equal(pending.buffer, tlsHTTP2ClientPreface[:len(pending.buffer)]) {
		return nil, true
	}
	if bytes.HasPrefix(pending.buffer, tlsHTTP2ClientPreface) {
		preface := append([]byte(nil), pending.buffer[:len(tlsHTTP2ClientPreface)]...)
		events = append(events, tlsHTTP2PrefaceEvent(pending.meta, preface))
		pending.buffer = pending.buffer[len(tlsHTTP2ClientPreface):]
	}

	for len(pending.buffer) >= tlsHTTP2FrameHeaderSize {
		header := pending.buffer[:tlsHTTP2FrameHeaderSize]
		if !validTLSHTTP2FrameHeader(header) {
			return events, false
		}
		frameLen := tlsHTTP2FrameLength(header)
		totalLen := tlsHTTP2FrameHeaderSize + frameLen
		if len(pending.buffer) < totalLen {
			break
		}
		frame := append([]byte(nil), pending.buffer[:totalLen]...)
		events = append(events, tlsHTTP2FrameEvent(pending.meta, frame, pending.flags))
		pending.buffer = pending.buffer[totalLen:]
		pending.flags = 0
	}

	if len(pending.buffer) > 0 && cap(pending.buffer) > 4*len(pending.buffer)+4096 {
		pending.buffer = append([]byte(nil), pending.buffer...)
	}
	return events, true
}

func tlsHTTP2PrefaceEvent(meta CompletedTLSFragment, payload []byte) TLSPlaintextEvent {
	return TLSPlaintextEvent{
		Type:         "http2_preface",
		Timestamp:    bpfKtimeToWallClock(meta.TimestampNS),
		PID:          meta.PID,
		TGID:         meta.TGID,
		Comm:         meta.Comm,
		Direction:    tlsDirectionLabel(meta.Direction),
		Lib:          libTypeName(meta.LibType),
		Function:     tlsFuncName(meta.Function),
		CapturedLen:  len(payload),
		OriginalLen:  len(payload),
		RawHexDump:   fmt.Sprintf("%x", payload),
		RawAvailable: true,
		BodySize:     len(payload),
		ContentType:  "application/http2",
	}
}

func tlsHTTP2FrameEvent(meta CompletedTLSFragment, frame []byte, flags uint8) TLSPlaintextEvent {
	frameType := byte(0xff)
	if len(frame) >= tlsHTTP2FrameHeaderSize {
		frameType = frame[3]
	}
	eventType := "http2_frame"
	body := ""
	contentType := "application/http2"
	bodySize := max(0, len(frame)-tlsHTTP2FrameHeaderSize)
	if frameType == 0x0 {
		eventType = "http2_data"
		payload := tlsHTTP2DataPayload(frame)
		bodySize = len(payload)
		if len(payload) > 0 && (utf8.Valid(payload) || looksLikeReadable(payload)) {
			body = string(bytes.TrimSpace(payload))
			if body != "" {
				contentType = "text/plain"
			}
		}
	}

	hexLen := min(len(frame), 512)
	return TLSPlaintextEvent{
		Type:         eventType,
		Timestamp:    bpfKtimeToWallClock(meta.TimestampNS),
		PID:          meta.PID,
		TGID:         meta.TGID,
		Comm:         meta.Comm,
		Direction:    tlsDirectionLabel(meta.Direction),
		Lib:          libTypeName(meta.LibType),
		Function:     tlsFuncName(meta.Function),
		CapturedLen:  len(frame),
		OriginalLen:  len(frame),
		Truncated:    flags&tlsFlagTruncated != 0,
		RawHexDump:   fmt.Sprintf("%x", frame[:hexLen]),
		RawAvailable: true,
		BodySize:     bodySize,
		Body:         body,
		ContentType:  contentType,
	}
}

func tlsHTTP2DataPayload(frame []byte) []byte {
	if len(frame) < tlsHTTP2FrameHeaderSize || frame[3] != 0x0 {
		return nil
	}
	payload := frame[tlsHTTP2FrameHeaderSize:]
	if frame[4]&0x8 == 0 { // PADDED
		return payload
	}
	if len(payload) == 0 {
		return nil
	}
	padding := int(payload[0])
	payload = payload[1:]
	if padding > len(payload) {
		return nil
	}
	return payload[:len(payload)-padding]
}

func tlsHTTP2FallbackEvent(meta CompletedTLSFragment, payload []byte, truncated bool) TLSPlaintextEvent {
	copyMeta := meta
	copyMeta.Payload = append([]byte(nil), payload...)
	copyMeta.TotalLen = uint32(len(payload))
	copyMeta.OriginalLen = uint32(len(payload))
	copyMeta.DataLen = uint32(len(payload))
	if truncated {
		copyMeta.Flags |= tlsFlagTruncated
	}
	event := completedToPlaintextEvent(copyMeta)
	if truncated && strings.HasPrefix(event.Type, "http2") {
		event.Type = "http2_truncated"
		event.Truncated = true
	}
	return event
}
