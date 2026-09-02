package tls

import (
	"bytes"
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

	// Keep the existing HTTP/1 assembler owned by the manager, but route all
	// completed transport records through the same upper-layer processor used by
	// bpf-ts. This removes a second copy of HTTP/1, HTTP/2, raw fallback, rules,
	// AgentSight dispatch, storage and broadcast semantics from the perf reader.
	processor := &tlsCompletedEventProcessor{
		http1:       httpStreams,
		http2:       NewTLSHTTP2StreamAssembler(10 * time.Second),
		store:       store,
		rules:       rules,
		broadcaster: broadcaster,
	}

	log.Printf("[tls] ReadLoop: started, waiting for perf events...")
	// TLS plaintext can arrive in bursts and each logical payload is split into
	// multiple perf records. A larger per-CPU buffer materially reduces ring
	// overwrites under concurrent agent traffic while remaining bounded.
	reader, err := perf.NewReader(events, os.Getpagesize()*256)
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

		if rec.LostSamples > 0 {
			m.readLoopStats.droppedFrags.Add(int64(rec.LostSamples))
			log.Printf("[tls] ReadLoop: kernel perf buffer lost %d samples", rec.LostSamples)
		}
		if len(rec.RawSample) == 0 {
			continue
		}

		totalFrags := m.readLoopStats.totalFrags.Add(1)
		m.readLoopStats.lastFragmentNS.Store(time.Now().UnixNano())
		if totalFrags <= 5 {
			log.Printf("[tls] ReadLoop: GOT fragment #%d raw_len=%d", totalFrags, len(rec.RawSample))
		}
		fragment, err := decodeTLSFragmentSample(rec.RawSample)
		if err != nil {
			m.readLoopStats.droppedFrags.Add(1)
			if totalFrags <= 5 {
				log.Printf("[tls] ReadLoop: fragment decode FAIL #%d (raw_len=%d): %v", totalFrags, len(rec.RawSample), err)
			}
			continue
		}
		observedPID := int(fragment.TGID)
		if observedPID <= 0 {
			observedPID = int(fragment.PID)
		}
		m.markTLSCaptureObserved(observedPID, time.Now().UnixNano())

		completed, ok := assembler.Add(fragment)
		if !ok || completed == nil {
			// A multi-fragment payload is expected to return incomplete until its
			// final fragment arrives. Do not count those normal states as drops.
			continue
		}
		m.readLoopStats.completedFrags.Add(1)

		result := processor.Process(*completed)
		if result.HTTPEvents > 0 {
			m.readLoopStats.httpEvents.Add(int64(result.HTTPEvents))
		}
		if result.RawEvents > 0 {
			m.readLoopStats.rawEvents.Add(int64(result.RawEvents))
		}

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

	if isTLSHTTP2Preface(f.Payload) {
		evType = "http2_preface"
		hexDump = fmt.Sprintf("%x", f.Payload[:min(len(f.Payload), 512)])
	} else if isTLSHTTP2Frame(f.Payload) {
		evType = "http2_frame"
		if len(f.Payload) >= tlsHTTP2FrameHeaderSize && f.Payload[3] == 0x0 {
			evType = "http2_data"
		}
		body = extractTLSHTTP2BodyText(f.Payload)
		if body != "" {
			contentType = "text/plain"
		} else {
			contentType = "application/http2"
		}
		hexDump = fmt.Sprintf("%x", f.Payload[:min(len(f.Payload), 512)])
	}

	if hexDump == "" && len(f.Payload) > 0 {
		hexDump = fmt.Sprintf("%x", f.Payload[:min(len(f.Payload), 512)])
	}

	return TLSPlaintextEvent{
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
}

func isTLSHTTP2Preface(data []byte) bool {
	return len(data) >= len(tlsHTTP2ClientPreface) && bytes.Equal(data[:len(tlsHTTP2ClientPreface)], tlsHTTP2ClientPreface)
}

func isTLSHTTP2Frame(data []byte) bool {
	if len(data) < tlsHTTP2FrameHeaderSize || !validTLSHTTP2FrameHeader(data[:tlsHTTP2FrameHeaderSize]) {
		return false
	}
	frameLen := tlsHTTP2FrameLength(data[:tlsHTTP2FrameHeaderSize])
	return len(data) >= tlsHTTP2FrameHeaderSize+frameLen
}

func extractTLSHTTP2BodyText(data []byte) string {
	if len(data) < tlsHTTP2FrameHeaderSize {
		return ""
	}
	var textParts []string
	offset := 0
	maxFrames := 32

	for frame := 0; frame < maxFrames && offset+tlsHTTP2FrameHeaderSize <= len(data); frame++ {
		header := data[offset : offset+tlsHTTP2FrameHeaderSize]
		if !validTLSHTTP2FrameHeader(header) {
			break
		}
		frameLen := tlsHTTP2FrameLength(header)
		totalLen := tlsHTTP2FrameHeaderSize + frameLen
		if offset+totalLen > len(data) {
			break
		}
		frameBytes := data[offset : offset+totalLen]
		if header[3] == 0x00 && frameLen > 0 {
			payload := tlsHTTP2DataPayload(frameBytes)
			if utf8.Valid(payload) {
				trimmed := bytes.TrimSpace(payload)
				if len(trimmed) > 0 {
					textParts = append(textParts, string(trimmed))
				}
			} else if looksLikeReadable(payload) {
				textParts = append(textParts, string(payload))
			}
		}
		offset += totalLen
	}

	return strings.Join(textParts, "\n")
}

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
