package main

import (
	"crypto/tls"
	"net/http"

	"agent-ebpf-filter/internal/domainforwardproxy"
)

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
