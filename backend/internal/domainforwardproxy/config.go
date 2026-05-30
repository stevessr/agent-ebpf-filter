package domainforwardproxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type domainForwardCertStore struct {
	defaultCert *tls.Certificate
	exact       map[string]*tls.Certificate
	wildcards   []domainForwardCertEntry
}

type domainForwardCertEntry struct {
	pattern string
	cert    *tls.Certificate
}

func NewTLSConfig(settings DomainForwardProxySettings) (*tls.Config, []string, error) {
	store := &domainForwardCertStore{exact: make(map[string]*tls.Certificate)}
	warnings := make([]string, 0)
	if strings.TrimSpace(settings.CertFile) != "" || strings.TrimSpace(settings.KeyFile) != "" {
		cert, err := tls.LoadX509KeyPair(settings.CertFile, settings.KeyFile)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("default certificate: %v", err))
		} else {
			store.defaultCert = &cert
		}
	}
	for _, route := range settings.Routes {
		if strings.TrimSpace(route.CertFile) == "" && strings.TrimSpace(route.KeyFile) == "" {
			continue
		}
		pattern := NormalizeDomainPattern(route.Host)
		if pattern == "" {
			continue
		}
		cert, err := tls.LoadX509KeyPair(route.CertFile, route.KeyFile)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("certificate for %s: %v", pattern, err))
			continue
		}
		certCopy := cert
		if strings.HasPrefix(pattern, "*.") {
			store.wildcards = append(store.wildcards, domainForwardCertEntry{pattern: pattern, cert: &certCopy})
			continue
		}
		store.exact[pattern] = &certCopy
	}
	sort.SliceStable(store.wildcards, func(i, j int) bool {
		return len(store.wildcards[i].pattern) > len(store.wildcards[j].pattern)
	})
	if store.defaultCert == nil && len(store.exact) == 0 && len(store.wildcards) == 0 {
		return nil, warnings, errors.New("https forwarding requires a default cert/key or route-level cert/key")
	}
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: store.GetCertificate,
	}, warnings, nil
}

func (s *domainForwardCertStore) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	host := ""
	if hello != nil {
		host = NormalizeForwardHost(hello.ServerName)
	}
	if host != "" {
		if cert, ok := s.exact[host]; ok {
			return cert, nil
		}
		for _, entry := range s.wildcards {
			suffix := strings.TrimPrefix(entry.pattern, "*.")
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				return entry.cert, nil
			}
		}
	}
	if s.defaultCert != nil {
		return s.defaultCert, nil
	}
	return nil, fmt.Errorf("no certificate configured for %q", host)
}

func NewTransport(settings DomainForwardProxySettings) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	timeout := time.Duration(settings.DialTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	resolverAddress := strings.TrimSpace(settings.DNSResolver)
	if resolverAddress == "" {
		transport.DialContext = dialer.DialContext
		return transport
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, resolverAddress)
		},
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || net.ParseIP(host) != nil {
			return dialer.DialContext(ctx, network, address)
		}
		ips, err := resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("resolver returned no address for %s", host)
	}
	return transport
}

func NormalizeSettings(settings *DomainForwardProxySettings) {
	if settings == nil {
		return
	}
	settings.HTTPPort = normalizeTCPPort(settings.HTTPPort, 80)
	settings.HTTPSPort = normalizeTCPPort(settings.HTTPSPort, 443)
	settings.DefaultScheme = normalizedForwardScheme(settings.DefaultScheme)
	settings.CertFile = strings.TrimSpace(settings.CertFile)
	settings.KeyFile = strings.TrimSpace(settings.KeyFile)
	settings.DNSResolver = normalizeDNSResolver(settings.DNSResolver)
	if settings.DialTimeoutSeconds <= 0 {
		settings.DialTimeoutSeconds = 10
	} else if settings.DialTimeoutSeconds > 120 {
		settings.DialTimeoutSeconds = 120
	}
	seen := make(map[string]struct{}, len(settings.Routes))
	routes := make([]DomainForwardRoute, 0, len(settings.Routes))
	for _, route := range settings.Routes {
		route.Host = NormalizeDomainPattern(route.Host)
		route.Upstream = strings.TrimSpace(route.Upstream)
		route.CertFile = strings.TrimSpace(route.CertFile)
		route.KeyFile = strings.TrimSpace(route.KeyFile)
		if route.Host == "" {
			continue
		}
		if _, ok := seen[route.Host]; ok {
			continue
		}
		seen[route.Host] = struct{}{}
		routes = append(routes, route)
	}
	settings.Routes = routes
}

func normalizeTCPPort(value int, fallback int) int {
	if value <= 0 || value > 65535 {
		return fallback
	}
	return value
}

func normalizedForwardScheme(value string) string {
	scheme := strings.ToLower(strings.TrimSpace(value))
	if scheme != "http" && scheme != "https" {
		return "https"
	}
	return scheme
}

func normalizeDNSResolver(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(value); err == nil {
		return value
	}
	trimmed := strings.Trim(value, "[]")
	if ip := net.ParseIP(trimmed); ip != nil {
		return net.JoinHostPort(ip.String(), "53")
	}
	if strings.Contains(value, ":") {
		return value
	}
	return net.JoinHostPort(value, "53")
}

func NormalizeDomainPattern(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(value, "*.") {
		suffix := NormalizeForwardHost(strings.TrimPrefix(value, "*."))
		if suffix == "" {
			return ""
		}
		return "*." + suffix
	}
	return NormalizeForwardHost(value)
}

func NormalizeForwardHost(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		if parsed, err := url.Parse(value); err == nil {
			value = parsed.Host
		}
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	} else if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
	}
	value = strings.TrimSuffix(value, ".")
	value = strings.Trim(value, "[]")
	return strings.ToLower(value)
}

func SettingsEqual(a, b DomainForwardProxySettings) bool {
	if a.Enabled != b.Enabled || a.HTTPPort != b.HTTPPort || a.HTTPSPort != b.HTTPSPort ||
		a.DefaultScheme != b.DefaultScheme || a.AllowAnyHost != b.AllowAnyHost ||
		a.DNSResolver != b.DNSResolver || a.DialTimeoutSeconds != b.DialTimeoutSeconds ||
		a.CertFile != b.CertFile || a.KeyFile != b.KeyFile || len(a.Routes) != len(b.Routes) {
		return false
	}
	for i := range a.Routes {
		if a.Routes[i] != b.Routes[i] {
			return false
		}
	}
	return true
}
