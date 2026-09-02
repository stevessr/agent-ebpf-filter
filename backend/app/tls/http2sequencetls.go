package tls

// acceptsFrameSequence enforces RFC 9113 header-block ordering using the
// decoder's existing pending block map. Once HEADERS or PUSH_PROMISE starts a
// block without END_HEADERS, the next frame on that connection/direction must
// be CONTINUATION for the same stream. No additional hot-path state is needed.
func (d *tlsHTTP2HeaderDecoder) acceptsFrameSequence(key tlsHTTP2StreamKey, frame []byte) bool {
	if d == nil || len(frame) < tlsHTTP2FrameHeaderSize {
		return true
	}

	frameType := frame[3]
	streamID := tlsHTTP2StreamID(frame[:tlsHTTP2FrameHeaderSize])
	var expectedStream uint32
	waiting := false
	if len(d.blocks) > 0 {
		for blockKey := range d.blocks {
			if blockKey.Stream == key {
				expectedStream = blockKey.ID
				waiting = true
				break
			}
		}
	}

	if waiting {
		return frameType == 0x9 && streamID == expectedStream
	}
	// A CONTINUATION without an outstanding block is invalid as well. Treat it
	// as a protocol/capture gap instead of letting compressed bytes reach the
	// generic frame path.
	return frameType != 0x9
}

func tlsHTTP2SequenceErrorEvent(event TLSPlaintextEvent) TLSPlaintextEvent {
	event.Type = "http2_headers_sequence_error"
	event.Body = ""
	event.BodySize = 0
	event.Headers = nil
	event.RawHexDump = ""
	event.RawAvailable = false
	event.RedactionState = "suppressed"
	event.DataType = "protocol_error"
	return event
}
