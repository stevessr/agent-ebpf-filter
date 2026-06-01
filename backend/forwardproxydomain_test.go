package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDomainForwardProxyRoutesByHost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "host=%s path=%s query=%s forwarded=%s route=%s", r.Host, r.URL.Path, r.URL.RawQuery, r.Header.Get("X-Forwarded-Host"), r.Header.Get("X-Agent-Forward-Route"))
	}))
	defer upstream.Close()

	handler := newDomainForwardProxyHandler(DomainForwardProxySettings{
		DefaultScheme: "http",
		Routes: []DomainForwardRoute{{
			Host:     "Example.TEST",
			Upstream: upstream.URL + "/base",
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.test/hello?x=1", nil)
	req.Host = "example.test"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"host=example.test",
		"path=/base/hello",
		"query=x=1",
		"forwarded=example.test",
		"route=example.test",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response %q does not contain %q", body, want)
		}
	}
}

func TestDomainForwardProxyRejectsUnknownHostUnlessAllowed(t *testing.T) {
	handler := newDomainForwardProxyHandler(DomainForwardProxySettings{
		DefaultScheme: "http",
		Routes:        []DomainForwardRoute{{Host: "known.test", Upstream: "http://127.0.0.1:1"}},
	})

	req := httptest.NewRequest(http.MethodGet, "http://unknown.test/", nil)
	req.Host = "unknown.test"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if !strings.Contains(rec.Body.String(), "no forwarding route") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestDomainForwardProxyAllowAnyHostBuildsHostTarget(t *testing.T) {
	handler := newDomainForwardProxyHandler(DomainForwardProxySettings{
		AllowAnyHost:  true,
		DefaultScheme: "https",
	})

	target, route, err := handler.targetForHost("Service.Example:443")
	if err != nil {
		t.Fatalf("targetForHost returned error: %v", err)
	}
	if got, want := target.String(), "https://service.example"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if route.Host != "service.example" {
		t.Fatalf("route host = %q", route.Host)
	}
}

func TestDomainForwardProxyWildcardAndHostPlaceholder(t *testing.T) {
	handler := newDomainForwardProxyHandler(DomainForwardProxySettings{
		DefaultScheme: "http",
		Routes: []DomainForwardRoute{{
			Host:     "*.example.test",
			Upstream: "http://upstream.internal/{host}",
		}},
	})

	target, route, err := handler.targetForHost("api.example.test")
	if err != nil {
		t.Fatalf("targetForHost returned error: %v", err)
	}
	if route.Host != "*.example.test" {
		t.Fatalf("route host = %q", route.Host)
	}
	if got, want := target.String(), "http://upstream.internal/api.example.test"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
}

func TestNormalizeDomainForwardProxySettingsDefaults(t *testing.T) {
	settings := DomainForwardProxySettings{
		HTTPPort:           -1,
		HTTPSPort:          70000,
		DefaultScheme:      "ftp",
		DNSResolver:        "1.1.1.1",
		DialTimeoutSeconds: 999,
		Routes: []DomainForwardRoute{
			{Host: "Example.Test:8443", Upstream: " backend:8080 "},
			{Host: "example.test", Upstream: "duplicate"},
			{Host: ""},
		},
	}
	normalizeDomainForwardProxySettings(&settings)

	if settings.HTTPPort != 80 || settings.HTTPSPort != 443 {
		t.Fatalf("ports = %d/%d, want 80/443", settings.HTTPPort, settings.HTTPSPort)
	}
	if settings.DefaultScheme != "https" {
		t.Fatalf("default scheme = %q", settings.DefaultScheme)
	}
	if settings.DNSResolver != "1.1.1.1:53" {
		t.Fatalf("dns resolver = %q", settings.DNSResolver)
	}
	if settings.DialTimeoutSeconds != 120 {
		t.Fatalf("dial timeout = %d", settings.DialTimeoutSeconds)
	}
	if len(settings.Routes) != 1 {
		t.Fatalf("route count = %d", len(settings.Routes))
	}
	if settings.Routes[0].Host != "example.test" || settings.Routes[0].Upstream != "backend:8080" {
		t.Fatalf("route = %+v", settings.Routes[0])
	}
}

func TestDomainForwardProxyStatusEndpointReturnsCopy(t *testing.T) {
	oldService := domainForwardProxyService
	service := newDomainForwardProxyRuntime()
	domainForwardProxyService = service
	t.Cleanup(func() { domainForwardProxyService = oldService })

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/system/domain-forward/status", nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	handleDomainForwardProxyStatus(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
