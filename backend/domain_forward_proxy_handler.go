package main

import (
	"net/http"
	"net/url"

	"agent-ebpf-filter/internal/domainforwardproxy"
)

type domainForwardProxyHandler struct {
	inner *domainforwardproxy.Handler
}

func newDomainForwardProxyHandler(settings DomainForwardProxySettings) *domainForwardProxyHandler {
	return &domainForwardProxyHandler{inner: domainforwardproxy.NewHandler(toInternalDomainForwardProxySettings(settings))}
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
