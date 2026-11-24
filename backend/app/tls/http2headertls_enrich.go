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
