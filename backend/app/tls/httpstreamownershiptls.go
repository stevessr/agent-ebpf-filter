package tls

import (
	"bytes"
	"time"
)

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

	a.mu.Lock()
	_, alreadyOwned := a.pending[key]
	if !alreadyOwned && !looksLikeTLSHTTPMessageStart(trimTLSHTTPMessageSeparators(fragment.Payload), fragment.Direction) {
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

	recognized := alreadyOwned || looksLikeTLSHTTPMessageStart(trimTLSHTTPMessageSeparators(fragment.Payload), fragment.Direction)
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
