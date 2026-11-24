package tls

import (
	"fmt"
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
