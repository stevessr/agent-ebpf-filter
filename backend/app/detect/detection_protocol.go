package detect

import (
	"agent-ebpf-filter/internal/protocoldetect"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section detection_protocol.go ----

type HTTPRequestInfo = protocoldetect.HTTPRequestInfo
type AppProtocol = protocoldetect.AppProtocol


func extractTLSSNI(data []byte) (string, string, error) {
	return protocoldetect.ExtractTLSSNI(data)
}

func extractHTTPRequest(data []byte) (*HTTPRequestInfo, error) {
	return protocoldetect.ExtractHTTPRequest(data)
}

func fingerprintProtocol(data []byte, dport uint32) AppProtocol {
	return protocoldetect.FingerprintProtocol(data, dport)
}

func extractSSHInfo(data []byte) (version string, software string, err error) {
	return protocoldetect.ExtractSSHInfo(data)
}

func extractDHCPInfo(data []byte) (string, string, error) {
	return protocoldetect.ExtractDHCPInfo(data)
}

func extractDNSQueries(data []byte) []string {
	return protocoldetect.ExtractDNSQueries(data)
}

func extractMDNSQueries(data []byte) []string {
	return protocoldetect.ExtractMDNSQueries(data)
}

func extractQUICSNI(data []byte) string {
	return protocoldetect.ExtractQUICSNI(data)
}

func extractTLSSNIFromHandshake(data []byte) (string, string, error) {
	return protocoldetect.ExtractTLSSNIFromHandshake(data)
}

func extractQUICVersion(data []byte) string {
	return protocoldetect.ExtractQUICVersion(data)
}

func extractNTPInfo(data []byte) (version string, stratum string) {
	return protocoldetect.ExtractNTPInfo(data)
}

func extractSNMPInfo(data []byte) (version string, community string) {
	return protocoldetect.ExtractSNMPInfo(data)
}

func extractNetBIOSInfo(data []byte) (name string, nsType string) {
	return protocoldetect.ExtractNetBIOSInfo(data)
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
	case AppProtoHTTP, AppProtoSSDP:
		if req, err := extractHTTPRequest(data); err == nil {
			entry.HTTPHost = req.Host
			entry.HTTPMethod = req.Method
		}
	case AppProtoSSH:
		if ver, soft, err := extractSSHInfo(data); err == nil {
			entry.SNI = soft
			entry.HTTPHost = ver
		}
	case AppProtoDHCP:
		if hostname, msgType, err := extractDHCPInfo(data); err == nil {
			entry.HTTPHost = hostname
			entry.SNI = msgType
		}
	case AppProtoDNS:
		if queries := extractDNSQueries(data); len(queries) > 0 {
			entry.HTTPHost = strings.Join(queries, ", ")
		}
	case AppProtomDNS, AppProtoLLMNR:
		if queries := extractMDNSQueries(data); len(queries) > 0 {
			entry.HTTPHost = strings.Join(queries, ", ")
		}
	case AppProtoQUIC:
		if sni := extractQUICSNI(data); sni != "" {
			entry.SNI = sni
		}
		if ver := extractQUICVersion(data); ver != "" {
			entry.ALPN = ver
		}
	case AppProtoNTP:
		if ver, str := extractNTPInfo(data); ver != "" {
			entry.SNI = str
			entry.ALPN = ver
		}
	case AppProtoSNMP:
		if ver, comm := extractSNMPInfo(data); ver != "" {
			entry.SNI = comm
			entry.ALPN = ver
		}
	case AppProtoNetBIOS:
		if name, nsType := extractNetBIOSInfo(data); name != "" {
			entry.HTTPHost = name
			entry.SNI = nsType
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
