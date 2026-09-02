package tls

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http2/hpack"
)

const (
	tlsHTTP2MaxHeaderBlock    = 256 * 1024
	tlsHTTP2MaxHeaderFields   = 128
	tlsHTTP2MaxHeaderBytes    = 64 * 1024
	tlsHTTP2MaxHeaderString   = 64 * 1024
	tlsHTTP2MaxHPACKTable     = 64 * 1024
	tlsHTTP2MaxDecoderStates  = 1024
	tlsHTTP2MaxHeaderBlocks   = 2048
	tlsHTTP2MaxLogicalStreams = 4096
)

type tlsHTTP2LogicalStreamKey struct {
	PID          uint32
	TGID         uint32
	ConnectionID uint64
	LibType      uint8
	StreamID     uint32
}

type tlsHTTP2HeaderBlockKey struct {
	Stream tlsHTTP2StreamKey
	ID     uint32
}

type tlsHTTP2PendingHeaderBlock struct {
	data             []byte
	frameType        byte
	promisedStreamID uint32
	lastSeen         time.Time
}

type tlsHTTP2HPACKDecoderState struct {
	decoder  *hpack.Decoder
	lastSeen time.Time
}

type tlsHTTP2LogicalStreamState struct {
	request             tlsHTTPRequestContext
	requestDirection    uint8
	requestContentType  string
	statusCode          int
	responseDirection   uint8
	responseContentType string
	lastSeen            time.Time
}

type tlsHTTP2HeaderDecoder struct {
	decoders map[tlsHTTP2StreamKey]*tlsHTTP2HPACKDecoderState
	blocks   map[tlsHTTP2HeaderBlockKey]*tlsHTTP2PendingHeaderBlock
	streams  map[tlsHTTP2LogicalStreamKey]*tlsHTTP2LogicalStreamState
	ttl      time.Duration
}

func newTLSHTTP2HeaderDecoder(frameTimeout time.Duration) *tlsHTTP2HeaderDecoder {
	ttl := 10 * frameTimeout
	if ttl < 5*time.Minute {
		ttl = 5 * time.Minute
	}
	return &tlsHTTP2HeaderDecoder{
		decoders: make(map[tlsHTTP2StreamKey]*tlsHTTP2HPACKDecoderState),
		blocks:   make(map[tlsHTTP2HeaderBlockKey]*tlsHTTP2PendingHeaderBlock),
		streams:  make(map[tlsHTTP2LogicalStreamKey]*tlsHTTP2LogicalStreamState),
		ttl:      ttl,
	}
}

func tlsHTTP2LogicalKey(key tlsHTTP2StreamKey, streamID uint32) tlsHTTP2LogicalStreamKey {
	return tlsHTTP2LogicalStreamKey{
		PID:          key.PID,
		TGID:         key.TGID,
		ConnectionID: key.ConnectionID,
		LibType:      key.LibType,
		StreamID:     streamID,
	}
}

func sameTLSHTTP2Connection(a tlsHTTP2StreamKey, b tlsHTTP2LogicalStreamKey) bool {
	return a.PID == b.PID &&
		a.TGID == b.TGID &&
		a.ConnectionID == b.ConnectionID &&
		a.LibType == b.LibType
}

func newTLSHTTP2HPACKDecoder() *hpack.Decoder {
	decoder := hpack.NewDecoder(4096, func(hpack.HeaderField) {})
	decoder.SetAllowedMaxDynamicTableSize(tlsHTTP2MaxHPACKTable)
	decoder.SetMaxStringLength(tlsHTTP2MaxHeaderString)
	return decoder
}

func (d *tlsHTTP2HeaderDecoder) decoderFor(key tlsHTTP2StreamKey, now time.Time) *tlsHTTP2HPACKDecoderState {
	if state := d.decoders[key]; state != nil {
		state.lastSeen = now
		return state
	}
	if len(d.decoders) >= tlsHTTP2MaxDecoderStates {
		d.evictOldestDecoder()
	}
	state := &tlsHTTP2HPACKDecoderState{decoder: newTLSHTTP2HPACKDecoder(), lastSeen: now}
	d.decoders[key] = state
	return state
}

func (d *tlsHTTP2HeaderDecoder) resetDirection(key tlsHTTP2StreamKey) {
	if d == nil {
		return
	}
	delete(d.decoders, key)
	for blockKey := range d.blocks {
		if blockKey.Stream == key {
			delete(d.blocks, blockKey)
		}
	}
}

func (d *tlsHTTP2HeaderDecoder) dropConnection(key tlsHTTP2StreamKey) {
	if d == nil {
		return
	}
	for decoderKey := range d.decoders {
		if decoderKey.PID == key.PID &&
			decoderKey.TGID == key.TGID &&
			decoderKey.ConnectionID == key.ConnectionID &&
			decoderKey.LibType == key.LibType {
			delete(d.decoders, decoderKey)
		}
	}
	for blockKey := range d.blocks {
		decoderKey := blockKey.Stream
		if decoderKey.PID == key.PID &&
			decoderKey.TGID == key.TGID &&
			decoderKey.ConnectionID == key.ConnectionID &&
			decoderKey.LibType == key.LibType {
			delete(d.blocks, blockKey)
		}
	}
	for streamKey := range d.streams {
		if sameTLSHTTP2Connection(key, streamKey) {
			delete(d.streams, streamKey)
		}
	}
}

func (d *tlsHTTP2HeaderDecoder) evictOldestDecoder() {
	var oldestKey tlsHTTP2StreamKey
	var oldest *tlsHTTP2HPACKDecoderState
	for key, state := range d.decoders {
		if oldest == nil || state.lastSeen.Before(oldest.lastSeen) {
			oldestKey = key
			oldest = state
		}
	}
	if oldest != nil {
		delete(d.decoders, oldestKey)
	}
}

func (d *tlsHTTP2HeaderDecoder) evictOldestBlock() {
	var oldestKey tlsHTTP2HeaderBlockKey
	var oldest *tlsHTTP2PendingHeaderBlock
	for key, block := range d.blocks {
		if oldest == nil || block.lastSeen.Before(oldest.lastSeen) {
			oldestKey = key
			oldest = block
		}
	}
	if oldest != nil {
		delete(d.blocks, oldestKey)
		delete(d.decoders, oldestKey.Stream)
	}
}

func (d *tlsHTTP2HeaderDecoder) evictOldestStream() {
	var oldestKey tlsHTTP2LogicalStreamKey
	var oldest *tlsHTTP2LogicalStreamState
	for key, stream := range d.streams {
		if oldest == nil || stream.lastSeen.Before(oldest.lastSeen) {
			oldestKey = key
			oldest = stream
		}
	}
	if oldest != nil {
		delete(d.streams, oldestKey)
	}
}

func (d *tlsHTTP2HeaderDecoder) cleanup(now time.Time) {
	if d == nil {
		return
	}
	for key, state := range d.decoders {
		if now.Sub(state.lastSeen) > d.ttl {
			delete(d.decoders, key)
		}
	}
	for key, block := range d.blocks {
		if now.Sub(block.lastSeen) > 30*time.Second {
			delete(d.blocks, key)
			// A missing tail may contain dynamic-table mutations. Forget this
			// direction rather than decoding later headers against stale state.
			delete(d.decoders, key.Stream)
		}
	}
	for key, stream := range d.streams {
		if now.Sub(stream.lastSeen) > d.ttl {
			delete(d.streams, key)
		}
	}
}

func tlsHTTP2FrameTypeName(frameType byte) string {
	switch frameType {
	case 0x0:
		return "data"
	case 0x1:
		return "headers"
	case 0x2:
		return "priority"
	case 0x3:
		return "rst_stream"
	case 0x4:
		return "settings"
	case 0x5:
		return "push_promise"
	case 0x6:
		return "ping"
	case 0x7:
		return "goaway"
	case 0x8:
		return "window_update"
	case 0x9:
		return "continuation"
	default:
		return "unknown"
	}
}

func tlsHTTP2HeadersFragment(frame []byte) ([]byte, error) {
	if len(frame) < tlsHTTP2FrameHeaderSize || frame[3] != 0x1 {
		return nil, fmt.Errorf("not an HTTP/2 HEADERS frame")
	}
	payload := frame[tlsHTTP2FrameHeaderSize:]
	offset := 0
	padding := 0
	if frame[4]&0x8 != 0 {
		if len(payload) == 0 {
			return nil, fmt.Errorf("padded HEADERS frame has no pad length")
		}
		padding = int(payload[0])
		offset++
	}
	if frame[4]&0x20 != 0 {
		offset += 5
	}
	if offset > len(payload) || padding > len(payload)-offset {
		return nil, fmt.Errorf("invalid HEADERS padding/priority length")
	}
	return payload[offset : len(payload)-padding], nil
}

func tlsHTTP2PushPromiseFragment(frame []byte) (uint32, []byte, error) {
	if len(frame) < tlsHTTP2FrameHeaderSize || frame[3] != 0x5 {
		return 0, nil, fmt.Errorf("not an HTTP/2 PUSH_PROMISE frame")
	}
	payload := frame[tlsHTTP2FrameHeaderSize:]
	offset := 0
	padding := 0
	if frame[4]&0x8 != 0 {
		if len(payload) == 0 {
			return 0, nil, fmt.Errorf("padded PUSH_PROMISE frame has no pad length")
		}
		padding = int(payload[0])
		offset++
	}
	if len(payload)-offset < 4 {
		return 0, nil, fmt.Errorf("PUSH_PROMISE frame missing promised stream id")
	}
	promisedID := binary.BigEndian.Uint32(payload[offset:offset+4]) & 0x7fffffff
	offset += 4
	if promisedID == 0 || padding > len(payload)-offset {
		return 0, nil, fmt.Errorf("invalid PUSH_PROMISE stream id/padding")
	}
	return promisedID, payload[offset : len(payload)-padding], nil
}

func (d *tlsHTTP2HeaderDecoder) appendHeaderBlock(key tlsHTTP2HeaderBlockKey, fragment []byte, frameType byte, promisedStreamID uint32, now time.Time) ([]byte, bool) {
	block := d.blocks[key]
	if block == nil {
		if len(d.blocks) >= tlsHTTP2MaxHeaderBlocks {
			d.evictOldestBlock()
		}
		block = &tlsHTTP2PendingHeaderBlock{
			frameType:        frameType,
			promisedStreamID: promisedStreamID,
			lastSeen:         now,
		}
		d.blocks[key] = block
	}
	if len(block.data)+len(fragment) > tlsHTTP2MaxHeaderBlock {
		delete(d.blocks, key)
		delete(d.decoders, key.Stream)
		return nil, false
	}
	block.data = append(block.data, fragment...)
	block.lastSeen = now
	return block.data, true
}

func (d *tlsHTTP2HeaderDecoder) decodeBlock(key tlsHTTP2StreamKey, block []byte, now time.Time) ([]hpack.HeaderField, bool, error) {
	state := d.decoderFor(key, now)
	fields := make([]hpack.HeaderField, 0, 16)
	totalBytes := 0
	truncated := false
	state.decoder.SetEmitFunc(func(field hpack.HeaderField) {
		fieldBytes := len(field.Name) + len(field.Value)
		if len(fields) >= tlsHTTP2MaxHeaderFields || totalBytes+fieldBytes > tlsHTTP2MaxHeaderBytes {
			truncated = true
			return
		}
		fields = append(fields, field)
		totalBytes += fieldBytes
	})
	written, err := state.decoder.Write(block)
	if err == nil && written != len(block) {
		err = fmt.Errorf("HPACK decoder consumed %d/%d bytes", written, len(block))
	}
	if err == nil {
		err = state.decoder.Close()
	}
	if err != nil {
		// Capture loss can desynchronize HPACK's dynamic table. Reset only this
		// connection/direction so later static/literal headers can recover.
		state.decoder = newTLSHTTP2HPACKDecoder()
		state.lastSeen = now
		return nil, false, err
	}
	state.lastSeen = now
	return fields, truncated, nil
}

func tlsHTTP2FieldsToMetadata(fields []hpack.HeaderField) (method, path, authority string, status int, headers map[string]string, contentType string) {
	regular := make(http.Header)
	for _, field := range fields {
		name := strings.ToLower(strings.TrimSpace(field.Name))
		switch name {
		case ":method":
			method = field.Value
		case ":path":
			path = field.Value
		case ":authority":
			authority = field.Value
		case ":status":
			if parsed, err := strconv.Atoi(field.Value); err == nil {
				status = parsed
			}
		default:
			if name != "" && !strings.HasPrefix(name, ":") {
				regular.Add(name, field.Value)
			}
		}
	}
	if authority == "" {
		authority = regular.Get("host")
	}
	contentType = regular.Get("content-type")
	return method, sanitizeTLSURL(path), sanitizeTLSInlineSecrets(authority), status, sanitizeTLSHeaders(regular), contentType
}

func oppositeTLSDirection(direction uint8) uint8 {
	if direction == tlsDirectionSend {
		return tlsDirectionRecv
	}
	return tlsDirectionSend
}

func (d *tlsHTTP2HeaderDecoder) applyStreamContext(key tlsHTTP2StreamKey, streamID uint32, event *TLSPlaintextEvent, now time.Time) *tlsHTTP2LogicalStreamState {
	logicalKey := tlsHTTP2LogicalKey(key, streamID)
	state := d.streams[logicalKey]
	if state == nil {
		return nil
	}
	state.lastSeen = now
	if event.Method == "" {
		event.Method = state.request.Method
	}
	if event.URL == "" {
		event.URL = state.request.URL
	}
	if event.Host == "" {
		event.Host = state.request.Host
	}

	// Method/path/authority describe the logical request and are useful on both
	// directions. Status and media type are directional: do not paint response
	// metadata onto request DATA in full-duplex streams.
	if state.statusCode != 0 && key.Direction == state.responseDirection {
		event.StatusCode = state.statusCode
	}
	switch {
	case state.request.Method != "" && key.Direction == state.requestDirection && state.requestContentType != "":
		event.ContentType = state.requestContentType
	case state.statusCode != 0 && key.Direction == state.responseDirection && state.responseContentType != "":
		event.ContentType = state.responseContentType
	}
	return state
}

func (d *tlsHTTP2HeaderDecoder) streamState(streamID uint32, key tlsHTTP2StreamKey, now time.Time) *tlsHTTP2LogicalStreamState {
	logicalKey := tlsHTTP2LogicalKey(key, streamID)
	state := d.streams[logicalKey]
	if state == nil {
		if len(d.streams) >= tlsHTTP2MaxLogicalStreams {
			d.evictOldestStream()
		}
		state = &tlsHTTP2LogicalStreamState{}
		d.streams[logicalKey] = state
	}
	state.lastSeen = now
	return state
}

func (d *tlsHTTP2HeaderDecoder) storeHeaderContext(key tlsHTTP2StreamKey, streamID uint32, event *TLSPlaintextEvent, now time.Time) *tlsHTTP2LogicalStreamState {
	state := d.streamState(streamID, key, now)
	if event.Method != "" {
		state.request = tlsHTTPRequestContext{Method: event.Method, URL: event.URL, Host: event.Host}
		state.requestDirection = key.Direction
		state.requestContentType = event.ContentType
	}
	if event.StatusCode != 0 {
		state.statusCode = event.StatusCode
		state.responseDirection = key.Direction
		state.responseContentType = event.ContentType
	}
	return state
}

func (d *tlsHTTP2HeaderDecoder) storePushPromiseContext(key tlsHTTP2StreamKey, promisedStreamID uint32, event *TLSPlaintextEvent, now time.Time) {
	state := d.streamState(promisedStreamID, key, now)
	state.request = tlsHTTPRequestContext{Method: event.Method, URL: event.URL, Host: event.Host}
	// PUSH_PROMISE carries the synthetic request on the same wire direction as
	// the future pushed response. Model its logical request as the opposite
	// direction so response status/media metadata remains directional.
	state.requestDirection = oppositeTLSDirection(key.Direction)
	state.requestContentType = event.ContentType
}

func (d *tlsHTTP2HeaderDecoder) enrichFrame(key tlsHTTP2StreamKey, event TLSPlaintextEvent, frame []byte) TLSPlaintextEvent {
	if d == nil || len(frame) < tlsHTTP2FrameHeaderSize {
		return event
	}
	now := time.Now()
	d.cleanup(now)

	frameType := frame[3]
	flags := frame[4]
	streamID := tlsHTTP2StreamID(frame[:tlsHTTP2FrameHeaderSize])
	event.HTTP2StreamID = streamID
	event.HTTP2FrameType = tlsHTTP2FrameTypeName(frameType)
	event.HTTP2Flags = flags
	if state := d.decoders[key]; state != nil {
		state.lastSeen = now
	}

	switch frameType {
	case 0x0: // DATA
		state := d.applyStreamContext(key, streamID, &event, now)
		if event.Body != "" {
			event.Body = sanitizeTLSBody(event.Body, event.ContentType)
			event.RedactionState = "sanitized"
		}
		// DATA is plaintext application content. Never expose a raw hex mirror
		// that could bypass the sanitized body representation.
		event.RawHexDump = ""
		event.RawAvailable = true
		if state != nil && state.statusCode > 0 && key.Direction == state.responseDirection && flags&0x1 != 0 {
			delete(d.streams, tlsHTTP2LogicalKey(key, streamID))
		}
		return event

	case 0x1: // HEADERS
		fragment, err := tlsHTTP2HeadersFragment(frame)
		if err != nil {
			event.Type = "http2_headers_invalid"
			event.RawHexDump = ""
			d.resetDirection(key)
			return event
		}
		blockKey := tlsHTTP2HeaderBlockKey{Stream: key, ID: streamID}
		delete(d.blocks, blockKey)
		if flags&0x4 == 0 {
			if _, ok := d.appendHeaderBlock(blockKey, fragment, 0x1, 0, now); !ok {
				event.Type = "http2_headers_truncated"
				event.Truncated = true
			}
			event.RawHexDump = ""
			return event
		}
		return d.decodeHeaderEvent(key, streamID, event, fragment, now)

	case 0x5: // PUSH_PROMISE
		promisedID, fragment, err := tlsHTTP2PushPromiseFragment(frame)
		if err != nil {
			event.Type = "http2_push_promise_invalid"
			event.RawHexDump = ""
			d.resetDirection(key)
			return event
		}
		blockKey := tlsHTTP2HeaderBlockKey{Stream: key, ID: streamID}
		delete(d.blocks, blockKey)
		if flags&0x4 == 0 {
			if _, ok := d.appendHeaderBlock(blockKey, fragment, 0x5, promisedID, now); !ok {
				event.Type = "http2_push_promise_truncated"
				event.Truncated = true
			}
			event.RawHexDump = ""
			return event
		}
		return d.decodePushPromiseEvent(key, promisedID, event, fragment, now)

	case 0x9: // CONTINUATION
		blockKey := tlsHTTP2HeaderBlockKey{Stream: key, ID: streamID}
		pending := d.blocks[blockKey]
		if pending == nil {
			event.RawHexDump = ""
			d.resetDirection(key)
			return event
		}
		block, ok := d.appendHeaderBlock(blockKey, frame[tlsHTTP2FrameHeaderSize:], pending.frameType, pending.promisedStreamID, now)
		if !ok {
			event.Type = "http2_headers_truncated"
			event.Truncated = true
			event.RawHexDump = ""
			return event
		}
		if flags&0x4 == 0 {
			event.RawHexDump = ""
			return event
		}
		blockCopy := append([]byte(nil), block...)
		frameType := pending.frameType
		promisedID := pending.promisedStreamID
		delete(d.blocks, blockKey)
		if frameType == 0x5 {
			return d.decodePushPromiseEvent(key, promisedID, event, blockCopy, now)
		}
		return d.decodeHeaderEvent(key, streamID, event, blockCopy, now)

	case 0x3: // RST_STREAM
		delete(d.streams, tlsHTTP2LogicalKey(key, streamID))
	}
	return event
}

func (d *tlsHTTP2HeaderDecoder) applyDecodedHeaderFields(event *TLSPlaintextEvent, fields []hpack.HeaderField, metadataTruncated bool) {
	method, path, authority, status, headers, contentType := tlsHTTP2FieldsToMetadata(fields)
	event.Method = method
	event.URL = path
	event.Host = authority
	event.StatusCode = status
	event.Headers = headers
	event.ContentType = contentType
	event.RedactionState = "sanitized"
	if metadataTruncated {
		event.Truncated = true
	}
}

func (d *tlsHTTP2HeaderDecoder) decodeHeaderEvent(key tlsHTTP2StreamKey, streamID uint32, event TLSPlaintextEvent, block []byte, now time.Time) TLSPlaintextEvent {
	fields, metadataTruncated, err := d.decodeBlock(key, block, now)
	// Header blocks may contain credentials as literal HPACK strings. Do not
	// publish the compressed bytes even if decoding failed.
	event.RawHexDump = ""
	event.RawAvailable = true
	if err != nil {
		event.Type = "http2_headers_decode_error"
		return event
	}

	d.applyDecodedHeaderFields(&event, fields, metadataTruncated)
	if event.Method != "" {
		event.Type = "http2_request"
		d.storeHeaderContext(key, streamID, &event, now)
	} else if event.StatusCode != 0 {
		event.Type = "http2_response"
		state := d.storeHeaderContext(key, streamID, &event, now)
		if state != nil {
			d.applyStreamContext(key, streamID, &event, now)
		}
	} else {
		event.Type = "http2_headers"
		d.applyStreamContext(key, streamID, &event, now)
	}

	if event.StatusCode != 0 && event.HTTP2Flags&0x1 != 0 {
		delete(d.streams, tlsHTTP2LogicalKey(key, streamID))
	}
	return event
}

func (d *tlsHTTP2HeaderDecoder) decodePushPromiseEvent(key tlsHTTP2StreamKey, promisedStreamID uint32, event TLSPlaintextEvent, block []byte, now time.Time) TLSPlaintextEvent {
	fields, metadataTruncated, err := d.decodeBlock(key, block, now)
	event.RawHexDump = ""
	event.RawAvailable = true
	event.Type = "http2_push_promise"
	event.HTTP2PromisedStreamID = promisedStreamID
	if err != nil {
		event.Type = "http2_push_promise_decode_error"
		return event
	}
	d.applyDecodedHeaderFields(&event, fields, metadataTruncated)
	event.StatusCode = 0
	if event.Method != "" {
		d.storePushPromiseContext(key, promisedStreamID, &event, now)
	}
	return event
}
