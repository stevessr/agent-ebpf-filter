package domainforwardproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrivacyLoopbackGatewayRoutesToExplicitUpstream(t *testing.T) {
	transportCalls := 0
	transport := testRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		transportCalls++
		if request.URL.Scheme != "https" || request.URL.Host != "api.openai.com" {
			t.Errorf("wrong upstream: %s", request.URL.String())
		}
		if request.Host != "api.openai.com" {
			t.Errorf("upstream Host: %q", request.Host)
		}
		if request.Header.Get("X-Forwarded-Host") != "127.0.0.1:8318" {
			t.Errorf("original inbound Host was lost")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader("ok")),
			ContentLength: 2,
		}, nil
	})
	settings := DomainForwardProxySettings{
		BindAddress: "127.0.0.1",
		HTTPSDisabled: true,
		DefaultScheme: "https",
		Routes: []DomainForwardRoute{{
			Host: "127.0.0.1",
			Upstream: "https://api.openai.com",
			UseUpstreamHost: true,
		}},
		Privacy: PrivacySettings{Enabled: true, Action: "block"},
	}
	h := NewHandlerWithTransport(settings, transport)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8318/v1/responses", strings.NewReader(`{"input":"hello"}`))
	request.Host = "127.0.0.1:8318"
	request.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	h.ServeHTTP(result, request)
	if result.Code != http.StatusOK || transportCalls != 1 {
		t.Fatalf("gateway status %d calls %d: %s", result.Code, transportCalls, result.Body.String())
	}
	modified := settings
	modified.BindAddress = "0.0.0.0"
	if SettingsEqual(settings, modified) {
		t.Fatal("listen address changes must restart proxy")
	}
	modified = settings
	modified.HTTPSDisabled = false
	if SettingsEqual(settings, modified) {
		t.Fatal("HTTPS listener changes must restart proxy")
	}
}
