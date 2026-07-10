package domainforwardproxy

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn testRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestRoutesByHost(t *testing.T) {
	callCount := 0
	transport := testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		if req.Method != http.MethodGet {
			t.Fatalf("method = %q, want GET", req.Method)
		}
		if got, want := req.URL.Scheme, "http"; got != want {
			t.Fatalf("scheme = %q, want %q", got, want)
		}
		if got, want := req.URL.Host, "upstream.test"; got != want {
			t.Fatalf("URL host = %q, want %q", got, want)
		}
		body := fmt.Sprintf("host=%s path=%s query=%s forwarded=%s proto=%s route=%s", req.Host, req.URL.Path, req.URL.RawQuery, req.Header.Get("X-Forwarded-Host"), req.Header.Get("X-Forwarded-Proto"), req.Header.Get("X-Agent-Forward-Route"))
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader(body)),
			ContentLength: int64(len(body)),
			Request:       req,
		}, nil
	})
	handler := NewHandlerWithTransport(DomainForwardProxySettings{
		DefaultScheme: "http",
		Routes: []DomainForwardRoute{{
			Host:     "Example.TEST",
			Upstream: "http://upstream.test/base",
		}},
	}, transport)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/hello?x=1", nil)
	req.Host = "example.test"
	req.Header.Set("X-Forwarded-Host", "forged.test")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Agent-Forward-Route", "forged-route")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if callCount != 1 {
		t.Fatalf("transport calls = %d, want 1", callCount)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"host=example.test",
		"path=/base/hello",
		"query=x=1",
		"forwarded=example.test",
		"proto=http",
		"route=example.test",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response %q does not contain %q", body, want)
		}
	}
}

func TestTransportErrorReturnsGenericBadGateway(t *testing.T) {
	const internalError = "dial tcp 10.0.0.8:8443: connection refused"
	handler := NewHandlerWithTransport(DomainForwardProxySettings{
		DefaultScheme: "https",
		Routes: []DomainForwardRoute{{
			Host:     "example.test",
			Upstream: "https://internal.service:8443",
		}},
	}, testRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New(internalError)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.Host = "example.test"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if body := rec.Body.String(); body != "upstream request failed\n" {
		t.Fatalf("body = %q, want generic upstream error", body)
	}
	if strings.Contains(rec.Body.String(), internalError) || strings.Contains(rec.Body.String(), "internal.service") {
		t.Fatalf("response leaked upstream details: %q", rec.Body.String())
	}
}

func TestInvalidUpstreamReturnsGenericBadGateway(t *testing.T) {
	handler := NewHandler(DomainForwardProxySettings{
		DefaultScheme: "https",
		Routes: []DomainForwardRoute{{
			Host:     "example.test",
			Upstream: "https://secret.internal/%zz",
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.Host = "example.test"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if body := rec.Body.String(); body != "forwarding route is invalid\n" {
		t.Fatalf("body = %q, want generic route error", body)
	}
	if strings.Contains(rec.Body.String(), "secret.internal") || strings.Contains(rec.Body.String(), "%zz") {
		t.Fatalf("response leaked invalid upstream details: %q", rec.Body.String())
	}
}

func TestRejectsUnknownHostUnlessAllowed(t *testing.T) {
	handler := NewHandler(DomainForwardProxySettings{
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

func TestAllowAnyHostBuildsHostTarget(t *testing.T) {
	handler := NewHandler(DomainForwardProxySettings{
		AllowAnyHost:  true,
		DefaultScheme: "https",
	})

	target, route, err := handler.TargetForHost("Service.Example:443")
	if err != nil {
		t.Fatalf("TargetForHost returned error: %v", err)
	}
	if got, want := target.String(), "https://service.example"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if route.Host != "service.example" {
		t.Fatalf("route host = %q", route.Host)
	}
}

func TestWildcardAndHostPlaceholder(t *testing.T) {
	handler := NewHandler(DomainForwardProxySettings{
		DefaultScheme: "http",
		Routes: []DomainForwardRoute{{
			Host:     "*.example.test",
			Upstream: "http://upstream.internal/{host}",
		}},
	})

	target, route, err := handler.TargetForHost("api.example.test")
	if err != nil {
		t.Fatalf("TargetForHost returned error: %v", err)
	}
	if route.Host != "*.example.test" {
		t.Fatalf("route host = %q", route.Host)
	}
	if got, want := target.String(), "http://upstream.internal/api.example.test"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
}

func TestNormalizeSettingsDefaults(t *testing.T) {
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
	NormalizeSettings(&settings)

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
