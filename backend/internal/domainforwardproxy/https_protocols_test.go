package domainforwardproxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"testing"
	"time"
)

func TestHTTPSProtocolNegotiation(t *testing.T) {
	certFile, keyFile := writeTestCA(t)
	serverTLS, warnings, err := NewTLSConfig(DomainForwardProxySettings{
		TLSInterceptEnabled: true,
		TLSInterceptAllowlist: "api.openai.com",
		TLSInterceptCACertFile: certFile,
		TLSInterceptCAKeyFile: keyFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) > 0 {
		t.Fatalf("unexpected TLS warnings: %v", warnings)
	}
	if serverTLS.MinVersion != tls.VersionTLS12 {
		t.Fatalf("unexpected TLS min version %x", serverTLS.MinVersion)
	}
	if !slices.Contains(serverTLS.NextProtos, "h2") || !slices.Contains(serverTLS.NextProtos, "http/1.1") {
		t.Fatalf("HTTPS ALPN protocols = %v", serverTLS.NextProtos)
	}
	protocols := proxyHTTPProtocols()
	if !protocols.HTTP1() || !protocols.HTTP2() {
		t.Fatal("HTTP/1.1 and HTTP/2 must both be enabled")
	}
	transport := NewTransport(DomainForwardProxySettings{DNSResolver: "127.0.0.1:5353"}).(*http.Transport)
	defer transport.CloseIdleConnections()
	if transport.Protocols == nil || !transport.Protocols.HTTP2() || !transport.Protocols.HTTP1() {
		t.Fatal("custom DNS transport must retain HTTP/2")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{
		TLSConfig: serverTLS,
		Protocols: proxyHTTPProtocols(),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, r.Proto)
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	defer server.Close()
	go func() { _ = server.Serve(tls.NewListener(listener, serverTLS)) }()

	pemBytes, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		t.Fatal("could not load test CA")
	}
	clientTransport := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		Protocols: proxyHTTPProtocols(),
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, listener.Addr().String())
		},
	}
	defer clientTransport.CloseIdleConnections()
	client := &http.Client{Transport: clientTransport, Timeout: 5 * time.Second}
	res, err := client.Get("https://api.openai.com/test")
	if err != nil {
		t.Fatalf("TLS/ALPN HTTP/2 request: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.ProtoMajor != 2 || string(body) != "HTTP/2.0" {
		t.Fatalf("negotiated protocol %s, body %q; want h2", res.Proto, body)
	}
}
