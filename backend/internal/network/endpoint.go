package network

import (
	"net"
	"net/netip"
	"net/url"
	"strings"
)

var (
	suspiciousPortStrings = map[string]struct{}{
		"4444":  {},
		"1337":  {},
		"31337": {},
		"6666":  {},
		"6667":  {},
	}

	suspiciousDomainSuffixes = []string{
		"ngrok.io",
		"serveo.net",
		"localhost.run",
		"pastebin.com",
		"termbin.com",
		"ix.io",
	}
)

// SplitEndpointHostPort extracts lowercase host and trimmed port from host:port,
// [ipv6]:port, URL, or bare host strings (including annotated "ip:port [SNI: ...]" values).
func SplitEndpointHostPort(endpoint string) (host, port string) {
	s := strings.TrimSpace(endpoint)
	if s == "" {
		return "", ""
	}
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.Host != "" {
			s = u.Host
		}
	}
	if h, p, err := net.SplitHostPort(s); err == nil {
		return strings.ToLower(strings.TrimSpace(h)), strings.TrimSpace(p)
	}
	if idx := strings.Index(s, " ["); idx > 0 {
		prefix := strings.TrimSpace(s[:idx])
		if h, p, err := net.SplitHostPort(prefix); err == nil {
			return strings.ToLower(strings.TrimSpace(h)), strings.TrimSpace(p)
		}
	}
	return strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")), ""
}

// IsLoopbackHost reports whether host names localhost/localhost. or a loopback IPv4/IPv6 address.
func IsLoopbackHost(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	if h == "" {
		return false
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	addr, err := parseNormalizedIP(h)
	if err != nil {
		return false
	}
	return addr.IsLoopback()
}

// IsLoopbackEndpoint reports whether an endpoint targets a loopback host.
func IsLoopbackEndpoint(endpoint string) bool {
	host, _ := SplitEndpointHostPort(endpoint)
	return IsLoopbackHost(host)
}

// IsLocalEndpoint reports whether endpoint refers to a local Unix socket,
// loopback host/IP, or unspecified local bind address (0.0.0.0 / ::).
func IsLocalEndpoint(endpoint string) bool {
	s := strings.TrimSpace(endpoint)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "unix:") {
		return true
	}
	host, _ := SplitEndpointHostPort(s)
	if IsLoopbackHost(host) {
		return true
	}
	addr, err := parseNormalizedIP(host)
	if err != nil {
		return false
	}
	return addr.IsUnspecified()
}

func parseNormalizedIP(host string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, err
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	return addr, nil
}

// IsSuspiciousPort reports whether a port string matches known reverse-shell/C2 defaults.
func IsSuspiciousPort(port string) bool {
	_, ok := suspiciousPortStrings[strings.TrimSpace(port)]
	return ok
}

// IsSuspiciousPortNumber reports whether a numeric port matches known reverse-shell/C2 defaults.
func IsSuspiciousPortNumber(port uint16) bool {
	switch port {
	case 4444, 1337, 31337, 6666, 6667:
		return true
	default:
		return false
	}
}

// IsSuspiciousDomainHost reports whether host matches known tunneling or paste exfiltration domains.
func IsSuspiciousDomainHost(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if splitHost, _, err := net.SplitHostPort(h); err == nil {
		h = strings.TrimSuffix(splitHost, ".")
	}
	if h == "" {
		return false
	}
	for _, suffix := range suspiciousDomainSuffixes {
		if h == suffix || strings.HasSuffix(h, "."+suffix) {
			return true
		}
	}
	return false
}

// IsSuspiciousEndpoint reports whether endpoint uses a suspicious port or tunneling/paste domain.
func IsSuspiciousEndpoint(endpoint string) bool {
	host, port := SplitEndpointHostPort(endpoint)
	return (port != "" && IsSuspiciousPort(port)) || IsSuspiciousDomainHost(host)
}
