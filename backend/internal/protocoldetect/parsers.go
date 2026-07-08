package protocoldetect

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// ── TLS SNI extraction (from rustnet dpi/https.rs) ────────────────────

// TLSSNI extracts the Server Name Indication from a TLS ClientHello.
// It handles TLS 1.0/1.1/1.2/1.3 ClientHello messages.
func ExtractTLSSNI(data []byte) (string, string, error) {
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

func ExtractHTTPRequest(data []byte) (*HTTPRequestInfo, error) {
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

func FingerprintProtocol(data []byte, dport uint32) AppProtocol {
	if len(data) < 4 {
		return AppProtoUnknown
	}

	// TLS ClientHello: 0x16 0x03 ...
	if data[0] == 0x16 && data[1] == 0x03 && (data[2] >= 0x00 && data[2] <= 0x04) {
		return AppProtoTLS
	}

	// QUIC: check for QUIC Initial packet (long header with version)
	// QUIC long header: top bit set, version field at offset 1
	if data[0]&0x80 != 0 && len(data) >= 6 {
		version := binary.BigEndian.Uint32(data[1:5])
		if isQUICVersion(version) {
			return AppProtoQUIC
		}
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

	// DHCP: BOOTP/DHCP flags
	if len(data) >= 2 {
		if data[0] == 0x01 || data[0] == 0x02 { // BOOTREQUEST or BOOTREPLY
			if len(data) >= 240 {
				magic := binary.BigEndian.Uint32(data[236:240])
				if magic == 0x63825363 { // DHCP magic cookie
					return AppProtoDHCP
				}
			}
		}
	}

	// NTP: 48-byte messages with LI/VN/Mode in byte 0
	if len(data) >= 48 {
		li := (data[0] >> 6) & 0x03
		vn := (data[0] >> 3) & 0x07
		mode := data[0] & 0x07
		if li <= 3 && vn >= 1 && vn <= 4 && mode >= 1 && mode <= 7 && (dport == 123 || dport == 12345) {
			return AppProtoNTP
		}
	}

	// SNMP: ASN.1 BER sequence 0x30 followed by length
	if data[0] == 0x30 && len(data) >= 3 {
		// SNMP version field is an INTEGER (0x02 0x01 0x00-0x03) after the sequence
		if data[2] == 0x02 && len(data) >= 5 && data[3] == 0x01 && data[4] <= 0x03 {
			if dport == 161 || dport == 162 {
				return AppProtoSNMP
			}
		}
	}

	// SSDP: HTTP-like NOTIFY or M-SEARCH over UDP
	if strings.HasPrefix(string(data), "NOTIFY * HTTP/") || strings.HasPrefix(string(data), "M-SEARCH * HTTP/") {
		return AppProtoSSDP
	}
	// Also handle HTTP 200 OK response for SSDP
	if strings.HasPrefix(string(data), "HTTP/1.1 200 OK") && (dport == 1900) {
		return AppProtoSSDP
	}

	// LLMNR: DNS-format queries on port 5355 (multicast 224.0.0.252)
	if dport == 5355 && len(data) >= 12 {
		flags := binary.BigEndian.Uint16(data[2:4])
		qr := (flags >> 15) & 1
		opcode := (flags >> 11) & 0x0f
		if qr == 0 && opcode == 0 { // query, standard query
			return AppProtoLLMNR
		}
	}

	// NetBIOS Name Service: port 137, first 2 bytes = transaction ID, then flags
	if dport == 137 && len(data) >= 12 {
		flags := binary.BigEndian.Uint16(data[2:4])
		opcode := (flags >> 11) & 0x0f
		// NetBIOS NS uses opcode 0 (query) and 5 (registration)
		if opcode == 0 || opcode == 5 {
			// Check for valid question section (NAME_TRN_ID + TYPE + CLASS)
			// NetBIOS names are 32-byte encoded labels
			nameLen := int(data[12])
			if nameLen > 0 && nameLen <= 32 {
				return AppProtoNetBIOS
			}
		}
	}

	// Port-based inference (lower priority than payload inspection)
	switch dport {
	case 443:
		return AppProtoTLS
	case 80, 8080:
		return AppProtoHTTP
	case 22:
		return AppProtoSSH
	case 53:
		return AppProtoDNS
	case 67, 68:
		return AppProtoDHCP
	case 5353:
		return AppProtomDNS
	case 5355:
		return AppProtoLLMNR
	case 1900:
		return AppProtoSSDP
	case 123:
		return AppProtoNTP
	case 161, 162:
		return AppProtoSNMP
	case 137:
		return AppProtoNetBIOS
	}

	return AppProtoUnknown
}

func isQUICVersion(v uint32) bool {
	switch v {
	case 0x00000001: // QUIC v1 (RFC 9000)
		return true
	case 0x51303539: // "Q039" (draft-23 equivalent)
		return true
	case 0xff000000 + 29: // draft-29
		return true
	default:
		// QUIC version negotiation uses 0x00000000
		return v == 0x00000000
	}
}
