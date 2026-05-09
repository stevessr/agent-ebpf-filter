package main

import (
	"encoding/binary"
	"fmt"
	"strings"

	"golang.org/x/net/dns/dnsmessage"
)

func extractSSHInfo(data []byte) (version string, software string, err error) {
	if len(data) < 9 {
		return "", "", fmt.Errorf("too short for SSH banner")
	}

	// SSH banner: "SSH-2.0-OpenSSH_9.6\r\n" or similar
	banner := string(data)
	if !strings.HasPrefix(banner, "SSH-") {
		return "", "", fmt.Errorf("not SSH")
	}

	// Find end of banner line
	end := findCRLF(data)
	if end < 0 {
		end = len(data)
	}

	parts := strings.SplitN(banner[:end], "-", 3)
	if len(parts) < 2 {
		return banner[:min(end, 20)], "", nil
	}

	version = "SSH-" + parts[1]
	if len(parts) >= 3 {
		// parts[2] contains software version, possibly with trailing comments
		software = strings.TrimSpace(parts[2])
		// Strip comments after space
		if idx := strings.Index(software, " "); idx > 0 {
			software = software[:idx]
		}
	}

	return version, software, nil
}

// ── DHCP protocol detection ───────────────────────────────────────────

func extractDHCPInfo(data []byte) (string, string, error) {
	if len(data) < 240 {
		return "", "", fmt.Errorf("too short for DHCP")
	}

	// DHCP message type from option 53
	msgType := data[0]
	typeNames := map[byte]string{
		1: "DHCPDISCOVER", 2: "DHCPOFFER", 3: "DHCPREQUEST",
		4: "DHCPDECLINE", 5: "DHCPACK", 6: "DHCPNAK",
		7: "DHCPRELEASE", 8: "DHCPINFORM",
	}
	typeName := typeNames[msgType]
	if typeName == "" {
		typeName = fmt.Sprintf("DHCP-%d", msgType)
	}

	// Extract hostname from option 12 (Host Name)
	hostname := ""
	if len(data) > 240 {
		options := data[240:]
		for i := 0; i < len(options)-2; {
			optCode := options[i]
			if optCode == 255 { // End
				break
			}
			if optCode == 0 { // Pad
				i++
				continue
			}
			if i+1 >= len(options) {
				break
			}
			optLen := int(options[i+1])
			if optCode == 12 && optLen > 0 && i+2+optLen <= len(options) {
				hostname = string(options[i+2 : i+2+optLen])
				break
			}
			i += 2 + optLen
		}
	}

	return hostname, typeName, nil
}

// ── mDNS query extraction ─────────────────────────────────────────────

func extractDNSQueries(data []byte) []string {
	var parser dnsmessage.Parser
	header, err := parser.Start(data)
	if err != nil || header.Response {
		return nil
	}
	queries := make([]string, 0, 4)
	for {
		question, err := parser.Question()
		if err == dnsmessage.ErrSectionDone {
			break
		}
		if err != nil {
			return nil
		}
		name := strings.TrimSuffix(question.Name.String(), ".")
		if name != "" {
			queries = append(queries, name)
		}
	}
	return queries
}

func extractMDNSQueries(data []byte) []string {
	if len(data) < 12 {
		return nil
	}

	// DNS header: ID(2) Flags(2) QDCOUNT(2) ANCOUNT(2) NSCOUNT(2) ARCOUNT(2)
	qdcount := int(binary.BigEndian.Uint16(data[4:6]))
	if qdcount == 0 || qdcount > 10 {
		return nil
	}

	queries := make([]string, 0, qdcount)
	offset := 12

	for q := 0; q < qdcount && offset < len(data); q++ {
		name, newOffset := parseDNSName(data, offset)
		if name != "" {
			queries = append(queries, name)
		}
		offset = newOffset + 4 // skip QTYPE(2) + QCLASS(2)
	}

	return queries
}

func parseDNSName(data []byte, offset int) (string, int) {
	if offset >= len(data) {
		return "", offset
	}

	parts := make([]string, 0)
	pos := offset

	for pos < len(data) {
		length := int(data[pos])
		if length == 0 {
			pos++
			break
		}
		// Handle pointer compression (top 2 bits = 11)
		if length&0xC0 == 0xC0 {
			if pos+1 >= len(data) {
				break
			}
			ptr := int(binary.BigEndian.Uint16(data[pos:pos+2]) & 0x3FFF)
			name, _ := parseDNSName(data, ptr)
			parts = append(parts, name)
			pos += 2
			break
		}
		if length > 63 || pos+1+length > len(data) {
			break
		}
		parts = append(parts, string(data[pos+1:pos+1+length]))
		pos += 1 + length
	}

	return strings.Join(parts, "."), pos
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── QUIC Initial packet parsing ───────────────────────────────────────

func extractQUICSNI(data []byte) string {
	// QUIC Initial packet structure (RFC 9000):
	// Header Form(1) | Fixed Bit(1) | Long Packet Type(2) | Reserved(2) | Packet Number Len(2)
	// Version(4) | DCID Len(1) | DCID(0..20) | SCID Len(1) | SCID(0..20)
	// Token Len(varint) | Token | Length(varint) | Protected Payload (CRYPTO frame)
	if len(data) < 20 {
		return ""
	}
	// Must be long header (top bit set)
	if data[0]&0x80 == 0 {
		return ""
	}
	// Long packet type must be 0 (Initial)
	packetType := (data[0] >> 4) & 0x03
	if packetType != 0 {
		return ""
	}
	// Skip version(4), dcid len + dcid, scid len + scid
	offset := 5 // after header byte + version
	if offset >= len(data) {
		return ""
	}
	dcidLen := int(data[offset])
	offset += 1 + dcidLen
	if offset >= len(data) {
		return ""
	}
	scidLen := int(data[offset])
	offset += 1 + scidLen
	if offset >= len(data) {
		return ""
	}
	// Skip token (varint length + token)
	tokenLen, varintBytes := readVarint(data[offset:])
	if varintBytes <= 0 || offset+varintBytes+int(tokenLen) >= len(data) {
		return ""
	}
	offset += varintBytes + int(tokenLen)
	// Read length (varint)
	if offset >= len(data) {
		return ""
	}
	payloadLen, varintBytes := readVarint(data[offset:])
	if varintBytes <= 0 {
		return ""
	}
	offset += varintBytes
	payloadEnd := offset + int(payloadLen)
	if payloadEnd > len(data) {
		payloadEnd = len(data)
	}
	// Parse protected payload for CRYPTO frame containing ClientHello
	// Look for TLS 0x16 (Handshake) content type inside the protected payload
	// The Initial packet protection removes a header but we can scan for ClientHello
	if offset+5 >= len(data) {
		return ""
	}
	// Try to find TLS ClientHello within the payload
	return scanForSNIInCryptoPayload(data[offset:payloadEnd])
}

func scanForSNIInCryptoPayload(payload []byte) string {
	if len(payload) < 50 {
		return ""
	}
	// Scan for TLS ClientHello pattern (0x01 = ClientHello type, followed by 3-byte length)
	for i := 0; i < len(payload)-10; i++ {
		if payload[i] == 0x01 {
			// Potential ClientHello
			remainingLen := len(payload) - i
			if remainingLen < 43 {
				continue
			}
			// Try to extract SNI from this offset
			sni, _, err := extractTLSSNIFromHandshake(payload[i:])
			if err == nil && sni != "" {
				return sni
			}
		}
	}
	return ""
}

func extractTLSSNIFromHandshake(handshake []byte) (string, string, error) {
	if len(handshake) < 43 {
		return "", "", fmt.Errorf("too short")
	}
	// Handshake: Type(1) + Length(3) + Version(2) + Random(32)
	offset := 1 + 3 + 2 + 32 // skip Type + Length + Version + Random
	if offset+1 > len(handshake) {
		return "", "", fmt.Errorf("truncated at session ID")
	}
	sessionIDLen := int(handshake[offset])
	offset += 1 + sessionIDLen
	if offset+2 > len(handshake) {
		return "", "", fmt.Errorf("truncated at cipher suites")
	}
	cipherSuitesLen := int(binary.BigEndian.Uint16(handshake[offset:]))
	offset += 2 + cipherSuitesLen
	if offset+1 > len(handshake) {
		return "", "", fmt.Errorf("truncated at compression")
	}
	compressionLen := int(handshake[offset])
	offset += 1 + compressionLen
	if offset+2 > len(handshake) {
		return "", "", fmt.Errorf("no extensions")
	}
	extensionsLen := int(binary.BigEndian.Uint16(handshake[offset:]))
	offset += 2
	endOffset := offset + extensionsLen
	if endOffset > len(handshake) {
		endOffset = len(handshake)
	}
	for offset+4 <= endOffset {
		extType := binary.BigEndian.Uint16(handshake[offset:])
		extLen := int(binary.BigEndian.Uint16(handshake[offset+2:]))
		offset += 4
		if offset+extLen > endOffset {
			break
		}
		if extType == 0x0000 && extLen >= 5 { // SNI
			sniListLen := int(binary.BigEndian.Uint16(handshake[offset+2:]))
			if sniListLen > 0 && 2+5+sniListLen <= extLen {
				sniType := handshake[offset+2+2]
				sniLen := int(binary.BigEndian.Uint16(handshake[offset+2+3:]))
				if sniType == 0 && sniLen > 0 && sniLen <= 253 {
					return string(handshake[offset+2+5 : offset+2+5+sniLen]), "", nil
				}
			}
		}
		offset += extLen
	}
	return "", "", fmt.Errorf("SNI not found")
}

func extractQUICVersion(data []byte) string {
	if len(data) < 5 {
		return ""
	}
	version := binary.BigEndian.Uint32(data[1:5])
	switch {
	case version == 0x00000001:
		return "QUIC v1"
	case version == 0x00000000:
		return "Version Negotiation"
	case (version & 0xFF000000) == 0xFF000000:
		return fmt.Sprintf("QUIC draft-%d", version&0x00FFFFFF)
	case version == 0x51303539:
		return "QUIC 39 (Faceb...)"
	default:
		return fmt.Sprintf("QUIC 0x%08x", version)
	}
}

func readVarint(data []byte) (uint64, int) {
	if len(data) < 1 {
		return 0, 0
	}
	first := data[0]
	switch {
	case first>>6 == 0:
		return uint64(first), 1
	case first>>6 == 1:
		if len(data) < 2 {
			return 0, 0
		}
		return uint64(binary.BigEndian.Uint16(data[:2]) & 0x3FFF), 2
	case first>>6 == 2:
		if len(data) < 4 {
			return 0, 0
		}
		return uint64(binary.BigEndian.Uint32(data[:4]) & 0x3FFFFFFF), 4
	default:
		if len(data) < 8 {
			return 0, 0
		}
		return binary.BigEndian.Uint64(data[:8]) & 0x3FFFFFFFFFFFFFFF, 8
	}
}

// ── NTP packet parsing ─────────────────────────────────────────────────

func extractNTPInfo(data []byte) (version string, stratum string) {
	if len(data) < 48 {
		return "", ""
	}
	li := (data[0] >> 6) & 0x03
	vn := (data[0] >> 3) & 0x07
	mode := data[0] & 0x07
	stratumVal := data[1]

	liNames := map[byte]string{0: "no-warning", 1: "leap-61", 2: "leap-59", 3: "alarm"}
	liStr := liNames[li]
	if liStr == "" {
		liStr = fmt.Sprintf("li-%d", li)
	}
	modeNames := map[byte]string{1: "symmetric-active", 2: "symmetric-passive", 3: "client", 4: "server", 5: "broadcast", 6: "control"}
	modeStr := modeNames[mode]
	if modeStr == "" {
		modeStr = fmt.Sprintf("mode-%d", mode)
	}
	stratumStr := "unspecified"
	switch {
	case stratumVal == 0:
		stratumStr = "kiss-o'-death"
	case stratumVal == 1:
		stratumStr = "primary"
	case stratumVal <= 15:
		stratumStr = fmt.Sprintf("secondary-%d", stratumVal)
	case stratumVal == 16:
		stratumStr = "unsynchronized"
	default:
		stratumStr = "reserved"
	}

	version = fmt.Sprintf("NTPv%d %s %s", vn, liStr, modeStr)
	stratum = stratumStr
	return
}

// ── SNMP packet parsing ────────────────────────────────────────────────

func extractSNMPInfo(data []byte) (version string, community string) {
	if len(data) < 3 || data[0] != 0x30 {
		return "", ""
	}
	// BER sequence; scan for version INTEGER and community OCTET STRING
	offset := 2 // skip 0x30 + length byte
	if offset >= len(data) {
		return "", ""
	}
	// Version: INTEGER 0x02 0x01 <version>
	if offset+3 <= len(data) && data[offset] == 0x02 && data[offset+1] == 0x01 {
		snmpVer := data[offset+2]
		switch snmpVer {
		case 0:
			version = "SNMPv1"
		case 1:
			version = "SNMPv2c"
		case 3:
			version = "SNMPv3"
		default:
			version = fmt.Sprintf("SNMPv%d", snmpVer)
		}
		offset += 3
	}
	// Community: OCTET STRING 0x04 <length> <data>
	if offset+2 <= len(data) && data[offset] == 0x04 {
		commLen := int(data[offset+1])
		if commLen > 0 && offset+2+commLen <= len(data) {
			community = string(data[offset+2 : offset+2+commLen])
		}
	}
	return
}

// ── NetBIOS Name Service parsing ──────────────────────────────────────

func extractNetBIOSInfo(data []byte) (name string, nsType string) {
	if len(data) < 50 {
		return "", ""
	}
	flags := binary.BigEndian.Uint16(data[2:4])
	opcode := (flags >> 11) & 0x0f

	opNames := map[uint16]string{0: "QUERY", 5: "REGISTRATION", 6: "RELEASE", 7: "WACK", 8: "REFRESH"}
	nsType = opNames[opcode]
	if nsType == "" {
		nsType = fmt.Sprintf("op-%d", opcode)
	}

	// Question section: NAME (34 bytes encoded) + TYPE(2) + CLASS(2)
	name = decodeNetBIOSName(data[12:44])
	return
}

func decodeNetBIOSName(encoded []byte) string {
	if len(encoded) < 34 {
		return ""
	}
	// NetBIOS name encoding: each byte pair encodes one character
	// First byte: length of the label (always 32 for the full name)
	// Then 32 bytes of encoded name (each character uses 2 bytes: A..P mapped to 0..15 per nibble)
	result := make([]byte, 0, 16)
	for i := 0; i < 32; i += 2 {
		if i+1 >= len(encoded) {
			break
		}
		// First half: (encoded[i] - 'A')
		// Second half: (encoded[i+1] - 'A')
		c1 := encoded[i]
		c2 := encoded[i+1]
		b := byte(0)
		if c1 >= 'A' && c1 <= 'P' {
			b |= (c1 - 'A') << 4
		}
		if c2 >= 'A' && c2 <= 'P' {
			b |= c2 - 'A'
		}
		if b == 0 {
			break
		}
		result = append(result, b)
	}
	// NetBIOS names are space-padded; trim them
	name := strings.TrimRight(string(result), " ")
	return name
}
