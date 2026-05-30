package main

import (
	"log"
	"net/http"

	"agent-ebpf-filter/internal/domainforwardproxy"
	"github.com/gin-gonic/gin"
)

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
