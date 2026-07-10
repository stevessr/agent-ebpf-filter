package app

import (
	"agent-ebpf-filter/internal/domainforwardproxy"
	"crypto/tls"
	"log"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section forwardproxydomain.go ----

type DomainForwardProxyStatus = domainforwardproxy.Status

type domainForwardProxyRuntime struct {
	inner *domainforwardproxy.Runtime
}

var domainForwardProxyService = newDomainForwardProxyRuntime()

func newDomainForwardProxyRuntime() *domainForwardProxyRuntime {
	return &domainForwardProxyRuntime{inner: domainforwardproxy.NewRuntime()}
}

func (m *domainForwardProxyRuntime) Activate() {
	m.inner.Activate()
}

func (m *domainForwardProxyRuntime) Close() {
	m.inner.Close()
}

func (m *domainForwardProxyRuntime) Status() DomainForwardProxyStatus {
	return m.inner.Status()
}

func (m *domainForwardProxyRuntime) Apply(settings DomainForwardProxySettings) error {
	return m.inner.Apply(toInternalDomainForwardProxySettings(settings))
}

func buildDomainForwardProxyStatus(settings DomainForwardProxySettings) DomainForwardProxyStatus {
	return domainforwardproxy.BuildStatus(toInternalDomainForwardProxySettings(settings))
}

func applyRuntimeDomainForwardProxy(settings RuntimeSettings) {
	if err := domainForwardProxyService.Apply(settings.DomainForwardProxy); err != nil {
		log.Printf("[DOMAIN-FORWARD] proxy configuration is not fully active: %v", err)
	}
}

func handleDomainForwardProxyStatus(c *gin.Context) {
	c.JSON(http.StatusOK, domainForwardProxyService.Status())
}

func toInternalDomainForwardProxySettings(settings DomainForwardProxySettings) domainforwardproxy.DomainForwardProxySettings {
	routes := make([]domainforwardproxy.DomainForwardRoute, 0, len(settings.Routes))
	for _, route := range settings.Routes {
		routes = append(routes, domainforwardproxy.DomainForwardRoute{
			Host:     route.Host,
			Upstream: route.Upstream,
			CertFile: route.CertFile,
			KeyFile:  route.KeyFile,
		})
	}
	return domainforwardproxy.DomainForwardProxySettings{
		Enabled:            settings.Enabled,
		HTTPPort:           settings.HTTPPort,
		HTTPSPort:          settings.HTTPSPort,
		DefaultScheme:      settings.DefaultScheme,
		AllowAnyHost:       settings.AllowAnyHost,
		DNSResolver:        settings.DNSResolver,
		DialTimeoutSeconds: settings.DialTimeoutSeconds,
		CertFile:           settings.CertFile,
		KeyFile:            settings.KeyFile,
		Routes:             routes,
	}
}

func fromInternalDomainForwardProxySettings(settings domainforwardproxy.DomainForwardProxySettings) DomainForwardProxySettings {
	routes := make([]DomainForwardRoute, 0, len(settings.Routes))
	for _, route := range settings.Routes {
		routes = append(routes, DomainForwardRoute{
			Host:     route.Host,
			Upstream: route.Upstream,
			CertFile: route.CertFile,
			KeyFile:  route.KeyFile,
		})
	}
	return DomainForwardProxySettings{
		Enabled:            settings.Enabled,
		HTTPPort:           settings.HTTPPort,
		HTTPSPort:          settings.HTTPSPort,
		DefaultScheme:      settings.DefaultScheme,
		AllowAnyHost:       settings.AllowAnyHost,
		DNSResolver:        settings.DNSResolver,
		DialTimeoutSeconds: settings.DialTimeoutSeconds,
		CertFile:           settings.CertFile,
		KeyFile:            settings.KeyFile,
		Routes:             routes,
	}
}

func fromInternalDomainForwardRoute(route domainforwardproxy.DomainForwardRoute) DomainForwardRoute {
	return DomainForwardRoute{
		Host:     route.Host,
		Upstream: route.Upstream,
		CertFile: route.CertFile,
		KeyFile:  route.KeyFile,
	}
}

func newDomainForwardTLSConfig(settings DomainForwardProxySettings) (*tls.Config, []string, error) {
	return domainforwardproxy.NewTLSConfig(toInternalDomainForwardProxySettings(settings))
}

func newDomainForwardTransport(settings DomainForwardProxySettings) http.RoundTripper {
	return domainforwardproxy.NewTransport(toInternalDomainForwardProxySettings(settings))
}

func normalizeDomainForwardProxySettings(settings *DomainForwardProxySettings) {
	if settings == nil {
		return
	}
	internal := toInternalDomainForwardProxySettings(*settings)
	domainforwardproxy.NormalizeSettings(&internal)
	*settings = fromInternalDomainForwardProxySettings(internal)
}

func normalizeDomainPattern(raw string) string {
	return domainforwardproxy.NormalizeDomainPattern(raw)
}

func normalizeForwardHost(raw string) string {
	return domainforwardproxy.NormalizeForwardHost(raw)
}

func domainForwardProxySettingsEqual(a, b DomainForwardProxySettings) bool {
	return domainforwardproxy.SettingsEqual(toInternalDomainForwardProxySettings(a), toInternalDomainForwardProxySettings(b))
}

type domainForwardProxyHandler struct {
	inner *domainforwardproxy.Handler
}

func newDomainForwardProxyHandler(settings DomainForwardProxySettings) *domainForwardProxyHandler {
	return newDomainForwardProxyHandlerWithTransport(settings, nil)
}

func newDomainForwardProxyHandlerWithTransport(settings DomainForwardProxySettings, transport http.RoundTripper) *domainForwardProxyHandler {
	return &domainForwardProxyHandler{inner: domainforwardproxy.NewHandlerWithTransport(toInternalDomainForwardProxySettings(settings), transport)}
}

func (h *domainForwardProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.inner.ServeHTTP(w, r)
}

func (h *domainForwardProxyHandler) targetForHost(host string) (*url.URL, DomainForwardRoute, error) {
	target, route, err := h.inner.TargetForHost(host)
	return target, fromInternalDomainForwardRoute(route), err
}

func parseForwardUpstream(raw string, defaultScheme string, requestHost string) (*url.URL, error) {
	return domainforwardproxy.ParseUpstream(raw, defaultScheme, requestHost)
}

func joinForwardProxyPath(target *url.URL, incoming *url.URL) (path, rawpath string) {
	return domainforwardproxy.JoinPath(target, incoming)
}

func requestForwardedProto(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
