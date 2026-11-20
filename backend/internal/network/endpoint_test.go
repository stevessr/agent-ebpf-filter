package network

import "testing"

func TestSplitEndpointHostPort(t *testing.T) {
	cases := []struct {
		in       string
		wantHost string
		wantPort string
	}{
		{"127.0.0.1:8080", "127.0.0.1", "8080"},
		{"[::1]:8080", "::1", "8080"},
		{"[::1]", "::1", ""},
		{"https://API.Example.COM:8443/v1/chat", "api.example.com", "8443"},
		{"203.0.113.10:443 [SNI: example.com]", "203.0.113.10", "443"},
		{"Localhost.", "localhost.", ""},
		{"   ", "", ""},
	}
	for _, tc := range cases {
		h, p := SplitEndpointHostPort(tc.in)
		if h != tc.wantHost || p != tc.wantPort {
			t.Fatalf("SplitEndpointHostPort(%q) = (%q, %q), want (%q, %q)", tc.in, h, p, tc.wantHost, tc.wantPort)
		}
	}
}

func TestLoopbackAndLocalClassification(t *testing.T) {
	for _, host := range []string{"localhost", "LOCALHOST.", "sub.localhost", "127.0.0.1", "127.1.2.3", "::1", "::ffff:127.0.0.1"} {
		if !IsLoopbackHost(host) {
			t.Fatalf("expected IsLoopbackHost(%q) = true", host)
		}
	}
	for _, host := range []string{"", "127.0.0.1.evil.com", "evil-localhost.com", "0.0.0.0", "8.8.8.8"} {
		if IsLoopbackHost(host) {
			t.Fatalf("expected IsLoopbackHost(%q) = false", host)
		}
	}
	if !IsLocalEndpoint("unix:/tmp/agent.sock") || !IsLocalEndpoint("/var/run/docker.sock") || !IsLocalEndpoint("0.0.0.0:8080") {
		t.Fatal("expected unix sockets and unspecified bind endpoints to be treated as local")
	}
	if !IsLoopbackEndpoint("[::1]:8888") || IsLoopbackEndpoint("https://127.0.0.1.evil.com:443") {
		t.Fatal("loopback endpoint classification mismatch")
	}
}

func TestSuspiciousEndpointClassification(t *testing.T) {
	if IsSuspiciousEndpoint("service.internal:44440") {
		t.Fatal("port 44440 must not substring-match 4444")
	}
	for _, ep := range []string{"attacker.ngrok.io:443", "203.0.113.10:4444", "https://pastebin.com/raw/abc"} {
		if !IsSuspiciousEndpoint(ep) {
			t.Fatalf("expected IsSuspiciousEndpoint(%q) = true", ep)
		}
	}
	if !IsSuspiciousPortNumber(31337) || IsSuspiciousPortNumber(443) {
		t.Fatal("unexpected IsSuspiciousPortNumber result")
	}
}
