package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// DomainForwardRoute maps one request host/SNI name to an optional upstream.
// When Upstream is empty, the proxy forwards to <defaultScheme>://<request-host>.
type DomainForwardRoute struct {
	Host     string `json:"host"`
	Upstream string `json:"upstream,omitempty"`
	CertFile string `json:"certFile,omitempty"`
	KeyFile  string `json:"keyFile,omitempty"`
}

// DomainForwardProxySettings controls the optional public HTTP/HTTPS reverse proxy.
// It is intentionally runtime-configurable but disabled by default.
type DomainForwardProxySettings struct {
	Enabled            bool                 `json:"enabled"`
	HTTPPort           int                  `json:"httpPort"`
	HTTPSPort          int                  `json:"httpsPort"`
	DefaultScheme      string               `json:"defaultScheme"`
	AllowAnyHost       bool                 `json:"allowAnyHost"`
	DNSResolver        string               `json:"dnsResolver,omitempty"`
	DialTimeoutSeconds int                  `json:"dialTimeoutSeconds"`
	CertFile           string               `json:"certFile,omitempty"`
	KeyFile            string               `json:"keyFile,omitempty"`
	Routes             []DomainForwardRoute `json:"routes,omitempty"`
}

type DomainForwardProxyStatus struct {
	Enabled      bool      `json:"enabled"`
	HTTPRunning  bool      `json:"httpRunning"`
	HTTPSRunning bool      `json:"httpsRunning"`
	HTTPAddress  string    `json:"httpAddress,omitempty"`
	HTTPSAddress string    `json:"httpsAddress,omitempty"`
	HTTPPort     int       `json:"httpPort"`
	HTTPSPort    int       `json:"httpsPort"`
	RouteCount   int       `json:"routeCount"`
	AllowAnyHost bool      `json:"allowAnyHost"`
	DNSResolver  string    `json:"dnsResolver,omitempty"`
	Errors       []string  `json:"errors,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type domainForwardProxyRuntime struct {
	mu          sync.Mutex
	active      bool
	httpServer  *http.Server
	httpsServer *http.Server
	settings    DomainForwardProxySettings
	status      DomainForwardProxyStatus
}

var domainForwardProxyService = newDomainForwardProxyRuntime()

func newDomainForwardProxyRuntime() *domainForwardProxyRuntime {
	settings := DomainForwardProxySettings{}
	normalizeDomainForwardProxySettings(&settings)
	return &domainForwardProxyRuntime{
		settings: settings,
		status: DomainForwardProxyStatus{
			Enabled:    false,
			HTTPPort:   settings.HTTPPort,
			HTTPSPort:  settings.HTTPSPort,
			UpdatedAt:  time.Now().UTC(),
			RouteCount: 0,
		},
	}
}

func (m *domainForwardProxyRuntime) Activate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = true
}

func (m *domainForwardProxyRuntime) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeLocked()
	m.active = false
	m.status.HTTPRunning = false
	m.status.HTTPSRunning = false
	m.status.HTTPAddress = ""
	m.status.HTTPSAddress = ""
	m.status.UpdatedAt = time.Now().UTC()
}

func (m *domainForwardProxyRuntime) Status() DomainForwardProxyStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := m.status
	if len(m.status.Errors) > 0 {
		status.Errors = append([]string(nil), m.status.Errors...)
	}
	return status
}

func (m *domainForwardProxyRuntime) Apply(settings DomainForwardProxySettings) error {
	normalizeDomainForwardProxySettings(&settings)

	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active {
		m.settings = settings
		m.status = buildDomainForwardProxyStatus(settings)
		return nil
	}
	if domainForwardProxySettingsEqual(m.settings, settings) {
		return nil
	}

	m.closeLocked()
	m.settings = settings
	m.status = buildDomainForwardProxyStatus(settings)
	if !settings.Enabled {
		return nil
	}

	handler := newDomainForwardProxyHandler(settings)
	if settings.HTTPPort > 0 {
		if err := m.startHTTPServerLocked(settings, handler); err != nil {
			m.status.Errors = append(m.status.Errors, fmt.Sprintf("http listener: %v", err))
		}
	}
	if settings.HTTPSPort > 0 {
		if err := m.startHTTPSServerLocked(settings, handler); err != nil {
			m.status.Errors = append(m.status.Errors, fmt.Sprintf("https listener: %v", err))
		}
	}
	m.status.UpdatedAt = time.Now().UTC()
	if !m.status.HTTPRunning && !m.status.HTTPSRunning {
		return errors.New(strings.Join(m.status.Errors, "; "))
	}
	return nil
}

func (m *domainForwardProxyRuntime) startHTTPServerLocked(settings DomainForwardProxySettings, handler http.Handler) error {
	addr := fmt.Sprintf(":%d", settings.HTTPPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	m.httpServer = server
	m.status.HTTPRunning = true
	m.status.HTTPAddress = listener.Addr().String()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[DOMAIN-FORWARD] HTTP proxy stopped: %v", err)
			m.setRuntimeError(fmt.Sprintf("http listener stopped: %v", err))
		}
	}()
	log.Printf("[DOMAIN-FORWARD] HTTP proxy listening on %s", listener.Addr())
	return nil
}

func (m *domainForwardProxyRuntime) startHTTPSServerLocked(settings DomainForwardProxySettings, handler http.Handler) error {
	tlsConfig, warnings, err := newDomainForwardTLSConfig(settings)
	m.status.Errors = append(m.status.Errors, warnings...)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf(":%d", settings.HTTPSPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	m.httpsServer = server
	m.status.HTTPSRunning = true
	m.status.HTTPSAddress = listener.Addr().String()
	tlsListener := tls.NewListener(listener, tlsConfig)
	go func() {
		if err := server.Serve(tlsListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[DOMAIN-FORWARD] HTTPS proxy stopped: %v", err)
			m.setRuntimeError(fmt.Sprintf("https listener stopped: %v", err))
		}
	}()
	log.Printf("[DOMAIN-FORWARD] HTTPS proxy listening on %s", listener.Addr())
	return nil
}

func (m *domainForwardProxyRuntime) closeLocked() {
	shutdown := func(server *http.Server) {
		if server == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[DOMAIN-FORWARD] failed to stop proxy listener: %v", err)
		}
	}
	shutdown(m.httpServer)
	shutdown(m.httpsServer)
	m.httpServer = nil
	m.httpsServer = nil
}

func (m *domainForwardProxyRuntime) setRuntimeError(message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.Errors = append(m.status.Errors, message)
	m.status.UpdatedAt = time.Now().UTC()
}

func buildDomainForwardProxyStatus(settings DomainForwardProxySettings) DomainForwardProxyStatus {
	return DomainForwardProxyStatus{
		Enabled:      settings.Enabled,
		HTTPPort:     settings.HTTPPort,
		HTTPSPort:    settings.HTTPSPort,
		RouteCount:   len(settings.Routes),
		AllowAnyHost: settings.AllowAnyHost,
		DNSResolver:  settings.DNSResolver,
		UpdatedAt:    time.Now().UTC(),
	}
}

func applyRuntimeDomainForwardProxy(settings RuntimeSettings) {
	if err := domainForwardProxyService.Apply(settings.DomainForwardProxy); err != nil {
		log.Printf("[DOMAIN-FORWARD] proxy configuration is not fully active: %v", err)
	}
}

func handleDomainForwardProxyStatus(c *gin.Context) {
	c.JSON(http.StatusOK, domainForwardProxyService.Status())
}

type domainForwardProxyHandler struct {
	settings  DomainForwardProxySettings
	transport http.RoundTripper
	exact     map[string]DomainForwardRoute
	wildcards []DomainForwardRoute
}

func newDomainForwardProxyHandler(settings DomainForwardProxySettings) *domainForwardProxyHandler {
	normalizeDomainForwardProxySettings(&settings)
	exact := make(map[string]DomainForwardRoute, len(settings.Routes))
	wildcards := make([]DomainForwardRoute, 0)
	for _, route := range settings.Routes {
		pattern := normalizeDomainPattern(route.Host)
		if pattern == "" {
			continue
		}
		route.Host = pattern
		if strings.HasPrefix(pattern, "*.") {
			wildcards = append(wildcards, route)
			continue
		}
		exact[pattern] = route
	}
	sort.SliceStable(wildcards, func(i, j int) bool {
		return len(wildcards[i].Host) > len(wildcards[j].Host)
	})
	return &domainForwardProxyHandler{
		settings:  settings,
		transport: newDomainForwardTransport(settings),
		exact:     exact,
		wildcards: wildcards,
	}
}

func (h *domainForwardProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := normalizeForwardHost(r.Host)
	if host == "" && r.URL != nil {
		host = normalizeForwardHost(r.URL.Host)
	}
	if host == "" {
		http.Error(w, "missing request host", http.StatusBadRequest)
		return
	}
	target, route, err := h.targetForHost(host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	proxy := &httputil.ReverseProxy{
		Director: func(out *http.Request) {
			out.URL.Scheme = target.Scheme
			out.URL.Host = target.Host
			out.URL.Path, out.URL.RawPath = joinForwardProxyPath(target, r.URL)
			if target.RawQuery == "" || out.URL.RawQuery == "" {
				out.URL.RawQuery = target.RawQuery + out.URL.RawQuery
			} else {
				out.URL.RawQuery = target.RawQuery + "&" + out.URL.RawQuery
			}
			out.Host = r.Host
			out.Header.Set("X-Forwarded-Host", r.Host)
			out.Header.Set("X-Forwarded-Proto", requestForwardedProto(r))
			out.Header.Set("X-Agent-Forward-Route", route.Host)
		},
		Transport: h.transport,
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			log.Printf("[DOMAIN-FORWARD] upstream %s for host %s failed: %v", target.String(), host, err)
			http.Error(w, fmt.Sprintf("upstream %s failed: %v", target.Host, err), http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
}

func (h *domainForwardProxyHandler) targetForHost(host string) (*url.URL, DomainForwardRoute, error) {
	host = normalizeForwardHost(host)
	if host == "" {
		return nil, DomainForwardRoute{}, errors.New("empty host")
	}
	if route, ok := h.lookupRoute(host); ok {
		target, err := parseForwardUpstream(route.Upstream, h.settings.DefaultScheme, host)
		return target, route, err
	}
	if h.settings.AllowAnyHost {
		route := DomainForwardRoute{Host: host}
		target, err := parseForwardUpstream("", h.settings.DefaultScheme, host)
		return target, route, err
	}
	return nil, DomainForwardRoute{}, fmt.Errorf("no forwarding route for host %q", host)
}

func (h *domainForwardProxyHandler) lookupRoute(host string) (DomainForwardRoute, bool) {
	if route, ok := h.exact[host]; ok {
		return route, true
	}
	for _, route := range h.wildcards {
		suffix := strings.TrimPrefix(route.Host, "*.")
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return route, true
		}
	}
	return DomainForwardRoute{}, false
}

func requestForwardedProto(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func parseForwardUpstream(raw string, defaultScheme string, requestHost string) (*url.URL, error) {
	defaultScheme = normalizedForwardScheme(defaultScheme)
	upstream := strings.TrimSpace(raw)
	if upstream == "" {
		upstream = defaultScheme + "://" + requestHost
	} else {
		upstream = strings.ReplaceAll(upstream, "{host}", requestHost)
		if !strings.Contains(upstream, "://") {
			upstream = defaultScheme + "://" + upstream
		}
	}
	target, err := url.Parse(upstream)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream %q: %w", raw, err)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf("unsupported upstream scheme %q", target.Scheme)
	}
	if strings.TrimSpace(target.Host) == "" {
		return nil, fmt.Errorf("upstream %q does not include a host", upstream)
	}
	return target, nil
}

func joinForwardProxyPath(target *url.URL, incoming *url.URL) (path, rawpath string) {
	if target == nil || incoming == nil {
		return "", ""
	}
	targetPath := target.EscapedPath()
	incomingPath := incoming.EscapedPath()
	if targetPath == "" {
		return incoming.Path, incoming.RawPath
	}
	joined := singleJoiningSlash(targetPath, incomingPath)
	if target.RawPath == "" && incoming.RawPath == "" {
		return singleJoiningSlash(target.Path, incoming.Path), ""
	}
	return joined, joined
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
}

type domainForwardCertStore struct {
	defaultCert *tls.Certificate
	exact       map[string]*tls.Certificate
	wildcards   []domainForwardCertEntry
}

type domainForwardCertEntry struct {
	pattern string
	cert    *tls.Certificate
}

func newDomainForwardTLSConfig(settings DomainForwardProxySettings) (*tls.Config, []string, error) {
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
		pattern := normalizeDomainPattern(route.Host)
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
		host = normalizeForwardHost(hello.ServerName)
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

func newDomainForwardTransport(settings DomainForwardProxySettings) http.RoundTripper {
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

func normalizeDomainForwardProxySettings(settings *DomainForwardProxySettings) {
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
		route.Host = normalizeDomainPattern(route.Host)
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

func normalizeDomainPattern(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(value, "*.") {
		suffix := normalizeForwardHost(strings.TrimPrefix(value, "*."))
		if suffix == "" {
			return ""
		}
		return "*." + suffix
	}
	return normalizeForwardHost(value)
}

func normalizeForwardHost(raw string) string {
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

func domainForwardProxySettingsEqual(a, b DomainForwardProxySettings) bool {
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
