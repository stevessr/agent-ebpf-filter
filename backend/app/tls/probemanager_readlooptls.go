package tls

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cilium/ebpf/perf"
)

func (m *TLSProbeManager) ReadLoop() error {
	if m == nil {
		log.Printf("[tls] ReadLoop: manager is nil")
		return nil
	}
	m.mu.Lock()
	if m.closed || m.objs == nil || m.objs.TlsEvents == nil || m.assembler == nil || m.httpStreams == nil || m.store == nil || m.broadcaster == nil {
		log.Printf("[tls] ReadLoop: component nil — closed=%v objs=%v events=%v asm=%v http=%v store=%v bcast=%v",
			m.closed, m.objs != nil, m.objs != nil && m.objs.TlsEvents != nil,
			m.assembler != nil, m.httpStreams != nil, m.store != nil, m.broadcaster != nil)
		m.mu.Unlock()
		return nil
	}
	events := m.objs.TlsEvents
	assembler := m.assembler
	httpStreams := m.httpStreams
	store := m.store
	broadcaster := m.broadcaster
	rules := m.rules
	m.mu.Unlock()

	log.Printf("[tls] ReadLoop: started, waiting for perf events...")
	reader, err := perf.NewReader(events, os.Getpagesize()*64)
	if err != nil {
		log.Printf("[tls] ReadLoop: perf.NewReader failed: %v", err)
		return err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = reader.Close()
		return nil
	}
	m.reader = reader
	m.mu.Unlock()
	defer func() {
		_ = reader.Close()
		m.mu.Lock()
		if m.reader == reader {
			m.reader = nil
		}
		m.mu.Unlock()
	}()

	for {
		rec, err := reader.Read()
		if err != nil {
			if errors.Is(err, perf.ErrClosed) {
				stats := m.readLoopStats.Snapshot()
				log.Printf("[tls] ReadLoop: perf reader closed, total=%d frags, %d dropped, %d completed, %d http, %d raw",
					stats.TotalFrags, stats.DroppedFrags, stats.CompletedFrags, stats.HTTPEvents, stats.RawEvents)
				return nil
			}
			log.Printf("[tls] ReadLoop: perf read error: %v", err)
			return err
		}
		totalFrags := m.readLoopStats.totalFrags.Add(1)
		m.readLoopStats.lastFragmentNS.Store(time.Now().UnixNano())
		if totalFrags <= 5 {
			log.Printf("[tls] ReadLoop: GOT fragment #%d raw_len=%d", totalFrags, len(rec.RawSample))
		}
		var fragment tlsFragment
		if err := binary.Read(bytes.NewReader(rec.RawSample), binary.LittleEndian, &fragment); err != nil {
			if totalFrags <= 5 {
				log.Printf("[tls] ReadLoop: binary.Read FAIL on fragment #%d (raw_len=%d): %v", totalFrags, len(rec.RawSample), err)
			}
			continue
		}
		completed, ok := assembler.Add(fragment)
		if !ok || completed == nil {
			m.readLoopStats.droppedFrags.Add(1)
			continue
		}
		m.readLoopStats.completedFrags.Add(1)
		parsedEvents := httpStreams.Add(*completed)
		// If HTTP parser produced nothing (HTTP/2, non-HTTP protocol, etc.),
		// still emit a raw event so the user sees captured data.
		if len(parsedEvents) == 0 {
			raw := completedToPlaintextEvent(*completed)
			if rules == nil || rules.Allows(raw) {
				broadcaster.Broadcast(raw)
				store.Add(raw)
				m.readLoopStats.rawEvents.Add(1)
			}
		} else {
			for _, event := range parsedEvents {
				if rules != nil && !rules.Allows(event) {
					continue
				}
				DispatchTLSAgentEvent(&event, tlsAgentLoopDetector, deps.Broadcast)
				store.Add(event)
				broadcaster.Broadcast(event)
				m.readLoopStats.httpEvents.Add(1)
			}
		}
		// Periodic summary every 100 fragments
		if totalFrags%100 == 0 {
			stats := m.readLoopStats.Snapshot()
			log.Printf("[tls] ReadLoop: %d frags, %d dropped, %d completed, %d http, %d raw",
				stats.TotalFrags, stats.DroppedFrags, stats.CompletedFrags, stats.HTTPEvents, stats.RawEvents)
		}
	}
}

// completedToPlaintextEvent converts a reassembled TLS fragment into a raw
// TLSPlaintextEvent without HTTP parsing. This ensures non-HTTP protocols
// (HTTP/2, gRPC, WebSocket, proprietary) still produce visible events.
func completedToPlaintextEvent(f CompletedTLSFragment) TLSPlaintextEvent {
	now := bpfKtimeToWallClock(f.TimestampNS)
	dir := "recv"
	if f.Direction == tlsDirectionSend {
		dir = "send"
	}

	evType := "tls_plaintext"
	body := ""
	contentType := ""
	hexDump := ""

	// HTTP/2 detection: check for connection preface or frame header magic
	if len(f.Payload) >= 24 {
		if isTLSHTTP2Preface(f.Payload) {
			evType = "http2_preface"
			hexDump = fmt.Sprintf("%x", f.Payload[:min(len(f.Payload), 512)])
		} else if isTLSHTTP2Frame(f.Payload) {
			evType = "http2_frame"
			// Try to extract readable content from HTTP/2 frames
			body = extractTLSHTTP2BodyText(f.Payload)
			if body != "" {
				contentType = "text/plain"
			}
			hexDump = fmt.Sprintf("%x", f.Payload[:min(len(f.Payload), 512)])
		}
	}

	if hexDump == "" && len(f.Payload) > 0 {
		hexDump = fmt.Sprintf("%x", f.Payload[:min(len(f.Payload), 512)])
	}

	ev := TLSPlaintextEvent{
		Type:         evType,
		Timestamp:    now,
		PID:          f.PID,
		TGID:         f.TGID,
		Comm:         f.Comm,
		Direction:    dir,
		Lib:          libTypeName(f.LibType),
		Function:     tlsFuncName(f.Function),
		CapturedLen:  int(f.TotalLen),
		OriginalLen:  int(f.OriginalLen),
		Truncated:    f.Flags&tlsFlagTruncated != 0,
		RawHexDump:   hexDump,
		RawAvailable: len(hexDump) > 0,
		BodySize:     len(f.Payload),
		Body:         body,
		ContentType:  contentType,
	}

	return ev
}

// HTTP/2 connection preface: "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
func isTLSHTTP2Preface(data []byte) bool {
	return len(data) >= 24 &&
		data[0] == 0x50 && data[1] == 0x52 && data[2] == 0x49 && // "PRI"
		data[3] == 0x20 && data[4] == 0x2a && data[5] == 0x20 && // " * "
		data[6] == 0x48 && data[7] == 0x54 && data[8] == 0x54 && // "HTT"
		data[9] == 0x50 && data[10] == 0x2f && data[11] == 0x32 // "P/2"
}

// HTTP/2 frame header: 3 bytes length + 1 byte type (0-9) + 1 byte flags + 4 bytes stream ID (MSB cleared)
func isTLSHTTP2Frame(data []byte) bool {
	if len(data) < 9 {
		return false
	}
	// Frame type must be 0x00-0x09 (DATA through CONTINUATION)
	frameType := data[3]
	if frameType > 0x09 {
		return false
	}
	// Stream ID top bit must be 0
	if data[5]&0x80 != 0 {
		return false
	}
	// Frame length should be plausible
	frameLen := int(data[0])<<16 | int(data[1])<<8 | int(data[2])
	if frameLen < 0 || frameLen > 16*1024*1024 {
		return false
	}
	// Additional heuristic: if we have a full frame header + some payload,
	// check that the length isn't wildly larger than the available data
	if len(data) >= 9 && frameLen > 0 && len(data) < 9+frameLen {
		// Partial frame — still valid HTTP/2
		return true
	}
	return true
}

// extractTLSHTTP2BodyText tries to extract readable text from HTTP/2 frames
// (primarily DATA frame payloads, skipping HEADERS frame HPACK encoding)
func extractTLSHTTP2BodyText(data []byte) string {
	if len(data) < 9 {
		return ""
	}
	var textParts []string
	offset := 0
	maxFrames := 32 // safety limit

	for frame := 0; frame < maxFrames && offset+9 <= len(data); frame++ {
		frameLen := int(data[offset])<<16 | int(data[offset+1])<<8 | int(data[offset+2])
		frameType := data[offset+3]
		offset += 9

		if offset+frameLen > len(data) {
			break
		}
		if frameLen < 0 {
			break
		}

		// DATA frame (0x00): extract payload if it looks like text/JSON
		if frameType == 0x00 && frameLen > 0 {
			payload := data[offset : offset+frameLen]
			// Try as UTF-8 text
			if utf8.Valid(payload) {
				trimmed := bytes.TrimSpace(payload)
				if len(trimmed) > 0 {
					textParts = append(textParts, string(trimmed))
				}
			} else if looksLikeReadable(payload) {
				textParts = append(textParts, string(payload))
			}
		}
		offset += frameLen
	}

	return strings.Join(textParts, "\n")
}

// looksLikeReadable checks if data is mostly printable ASCII
func looksLikeReadable(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	printable := 0
	for _, b := range data {
		if (b >= 0x20 && b < 0x7f) || b == 0x0a || b == 0x0d || b == 0x09 {
			printable++
		}
	}
	return float64(printable)/float64(len(data)) > 0.7
}

func libTypeName(lib uint8) string {
	switch lib {
	case tlsLibOpenSSL:
		return "openssl"
	case tlsLibGo:
		return "go"
	case tlsLibGnuTLS:
		return "gnutls"
	case tlsLibNSS:
		return "nss"
	case tlsLibRustls:
		return "rustls"
	default:
		return "unknown"
	}
}

// findLoadedSSLLibraries reads /proc/<pid>/maps and returns paths of loaded
// .so files that match known SSL/TLS library names (libssl, libgnutls, libnspr, etc.).
