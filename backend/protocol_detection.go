package main

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ── TLS SNI extraction (from rustnet dpi/https.rs) ────────────────────

// TLSSNI extracts the Server Name Indication from a TLS ClientHello.
// It handles TLS 1.0/1.1/1.2/1.3 ClientHello messages.
func extractTLSSNI(data []byte) (string, string, error) {
	// data = []byte of initial TCP payload
	if len(data) < 43 {
		return "", "", fmt.Errorf("too short for TLS ClientHello")
	}

	// TLS Record: ContentType(1) + Version(2) + Length(2) + Payload
	if data[0] != 0x16 { // Handshake
		return "", "", fmt.Errorf("not a TLS handshake record")
	}

	tlsVersion := binary.BigEndian.Uint16(data[1:3])
	_ = tlsVersion
	recordLen := int(binary.BigEndian.Uint16(data[3:5]))
	if 5+recordLen > len(data) {
		recordLen = len(data) - 5
	}

	payload := data[5 : 5+recordLen]
	if len(payload) < 38 {
		return "", "", fmt.Errorf("handshake payload too short")
	}

	// Handshake: Type(1) + Length(3) + Version(2) + Random(32) + SessionID
	if payload[0] != 0x01 { // ClientHello
		return "", "", fmt.Errorf("not a ClientHello")
	}

	offset := 1 + 3 // skip Type + Length
	if offset+2 > len(payload) {
		return "", "", fmt.Errorf("truncated ClientHello")
	}
	clientVersion := binary.BigEndian.Uint16(payload[offset:])
	_ = clientVersion
	offset += 2 + 32 // skip Version + Random

	if offset+1 > len(payload) {
		return "", "", fmt.Errorf("truncated at session ID")
	}
	sessionIDLen := int(payload[offset])
	offset += 1 + sessionIDLen

	if offset+2 > len(payload) {
		return "", "", fmt.Errorf("truncated at cipher suites")
	}
	cipherSuitesLen := int(binary.BigEndian.Uint16(payload[offset:]))
	offset += 2 + cipherSuitesLen

	if offset+1 > len(payload) {
		return "", "", fmt.Errorf("truncated at compression")
	}
	compressionLen := int(payload[offset])
	offset += 1 + compressionLen

	if offset+2 > len(payload) {
		return "", "", fmt.Errorf("no extensions")
	}
	extensionsLen := int(binary.BigEndian.Uint16(payload[offset:]))
	offset += 2
	endOffset := offset + extensionsLen
	if endOffset > len(payload) {
		endOffset = len(payload)
	}

	sni := ""
	alpn := ""

	// Parse extensions for SNI (type 0x0000) and ALPN (type 0x0010)
	for offset+4 <= endOffset {
		extType := binary.BigEndian.Uint16(payload[offset:])
		extLen := int(binary.BigEndian.Uint16(payload[offset+2:]))
		offset += 4

		if offset+extLen > endOffset {
			break
		}

		switch extType {
		case 0x0000: // SNI
			if extLen >= 5 {
				sniListLen := int(binary.BigEndian.Uint16(payload[offset+2:]))
				if sniListLen > 0 && 2+5+sniListLen <= extLen {
					sniType := payload[offset+2+2]
					sniLen := int(binary.BigEndian.Uint16(payload[offset+2+3:]))
					if sniType == 0 && sniLen > 0 && sniLen <= 253 {
						sniBytes := payload[offset+2+5 : offset+2+5+sniLen]
						sni = string(sniBytes)
					}
				}
			}

		case 0x0010: // ALPN
			if extLen >= 5 {
				alpnListLen := int(binary.BigEndian.Uint16(payload[offset+2:]))
				if alpnListLen > 0 {
					alpnOffset := offset + 2 + 2
					alpnEnd := alpnOffset + alpnListLen
					protocols := make([]string, 0)
					for alpnOffset+1 <= alpnEnd && alpnOffset+1+int(payload[alpnOffset]) <= alpnEnd {
						protoLen := int(payload[alpnOffset])
						proto := string(payload[alpnOffset+1 : alpnOffset+1+protoLen])
						protocols = append(protocols, proto)
						alpnOffset += 1 + protoLen
					}
					alpn = strings.Join(protocols, ", ")
				}
			}
		}

		offset += extLen
	}

	if sni == "" {
		return "", "", fmt.Errorf("SNI not found")
	}

	return sni, alpn, nil
}

// ── HTTP request parsing (from rustnet dpi/http.rs) ──────────────────

type HTTPRequestInfo struct {
	Method string
	Host   string
	Path   string
	Agent  string
}

func extractHTTPRequest(data []byte) (*HTTPRequestInfo, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("too short for HTTP request")
	}

	// Parse first line: "METHOD /path HTTP/1.x\r\n"
	firstLineEnd := findCRLF(data)
	if firstLineEnd < 0 || firstLineEnd < 8 {
		return nil, fmt.Errorf("invalid request line")
	}

	firstLine := string(data[:firstLineEnd])
	parts := strings.Fields(firstLine)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid request line parts")
	}

	info := &HTTPRequestInfo{
		Method: strings.ToUpper(parts[0]),
		Path:   parts[1],
	}

	// Parse headers for Host and User-Agent
	headers := data[firstLineEnd+2:]
	for len(headers) > 0 {
		lineEnd := findCRLF(headers)
		if lineEnd < 0 {
			break
		}
		if lineEnd == 0 {
			break // empty line = end of headers
		}
		line := string(headers[:lineEnd])
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "host:"):
			info.Host = strings.TrimSpace(line[5:])
		case strings.HasPrefix(lower, "user-agent:"):
			info.Agent = strings.TrimSpace(line[11:])
		}
		headers = headers[lineEnd+2:]
	}

	if info.Host == "" {
		return info, fmt.Errorf("no Host header")
	}

	return info, nil
}

func findCRLF(data []byte) int {
	for i := 0; i < len(data)-1; i++ {
		if data[i] == '\r' && data[i+1] == '\n' {
			return i
		}
	}
	return -1
}

// ── Application protocol fingerprinting ───────────────────────────────

type AppProtocol string

const (
	AppProtoTLS    AppProtocol = "TLS"
	AppProtoHTTP   AppProtocol = "HTTP"
	AppProtoSSH    AppProtocol = "SSH"
	AppProtoDNS    AppProtocol = "DNS"
	AppProtoQUIC   AppProtocol = "QUIC"
	AppProtoUnknown AppProtocol = "Unknown"
)

func fingerprintProtocol(data []byte, dport uint32) AppProtocol {
	if len(data) < 4 {
		return AppProtoUnknown
	}

	// TLS ClientHello: 0x16 0x03 ...
	if data[0] == 0x16 && data[1] == 0x03 && (data[2] >= 0x00 && data[2] <= 0x04) {
		return AppProtoTLS
	}

	// HTTP: starts with method
	if len(data) >= 4 {
		methods := []string{"GET ", "POST", "PUT ", "HEAD", "DELE", "OPTI", "CONN", "TRAC", "PATC"}
		for _, m := range methods {
			if strings.HasPrefix(string(data[:len(m)]), m) {
				return AppProtoHTTP
			}
		}
	}

	// SSH: starts with "SSH-"
	if len(data) >= 4 && string(data[:4]) == "SSH-" {
		return AppProtoSSH
	}

	// Port-based inference
	switch dport {
	case 443:
		return AppProtoTLS
	case 80, 8080:
		return AppProtoHTTP
	case 22:
		return AppProtoSSH
	case 53:
		return AppProtoDNS
	}

	return AppProtoUnknown
}

// ── Protocol detection cache ──────────────────────────────────────────

type protoDetectionEntry struct {
	AppProtocol AppProtocol
	SNI         string
	ALPN        string
	HTTPHost    string
	HTTPMethod  string
	DetectedAt  time.Time
}

type protoDetectionCache struct {
	mu      sync.RWMutex
	entries map[string]*protoDetectionEntry // key: "dstIP:dstPort"
}

func newProtoDetectionCache() *protoDetectionCache {
	return &protoDetectionCache{
		entries: make(map[string]*protoDetectionEntry),
	}
}

func (c *protoDetectionCache) Record(key string, protocol AppProtocol, sni, alpn, httpHost, httpMethod string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &protoDetectionEntry{
		AppProtocol: protocol,
		SNI:         sni,
		ALPN:        alpn,
		HTTPHost:    httpHost,
		HTTPMethod:  httpMethod,
		DetectedAt:  time.Now().UTC(),
	}
}

func (c *protoDetectionCache) Lookup(key string) (*protoDetectionEntry, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Since(entry.DetectedAt) > 30*time.Minute {
		return nil, false
	}
	return entry, true
}

var protoCache = newProtoDetectionCache()

// detectAndRecordProtocol inspects event data for protocol signatures
// and records any detected protocol info.
func detectAndRecordProtocol(dstIP string, dstPort uint32, data []byte) *protoDetectionEntry {
	if len(data) == 0 || dstIP == "" || dstPort == 0 {
		return nil
	}

	appProto := fingerprintProtocol(data, dstPort)
	if appProto == AppProtoUnknown {
		return nil
	}

	entry := &protoDetectionEntry{
		AppProtocol: appProto,
	}

	switch appProto {
	case AppProtoTLS:
		if sni, alpn, err := extractTLSSNI(data); err == nil {
			entry.SNI = sni
			entry.ALPN = alpn
		}
	case AppProtoHTTP:
		if req, err := extractHTTPRequest(data); err == nil {
			entry.HTTPHost = req.Host
			entry.HTTPMethod = req.Method
		}
	}

	key := fmt.Sprintf("%s:%d", dstIP, dstPort)
	protoCache.Record(key, appProto, entry.SNI, entry.ALPN, entry.HTTPHost, entry.HTTPMethod)

	return entry
}

// enrichEndpointWithProtocol enhances an endpoint string with protocol info.
func enrichEndpointWithProtocol(endpoint string) string {
	host, _, err := splitEndpointHostPort(endpoint)
	if err != nil {
		return endpoint
	}
	// Check cache for protocol detection
	entry, ok := protoCache.Lookup(host + ":443")
	if !ok {
		entry, ok = protoCache.Lookup(host + ":80")
	}
	if ok {
		if entry.SNI != "" {
			return fmt.Sprintf("%s [SNI: %s]", endpoint, entry.SNI)
		}
		if entry.HTTPHost != "" {
			return fmt.Sprintf("%s [Host: %s]", endpoint, entry.HTTPHost)
		}
	}
	return endpoint
}

func splitEndpointHostPort(endpoint string) (string, string, error) {
	for i := len(endpoint) - 1; i >= 0; i-- {
		if endpoint[i] == ':' {
			return endpoint[:i], endpoint[i+1:], nil
		}
	}
	return endpoint, "", fmt.Errorf("no port")
}
