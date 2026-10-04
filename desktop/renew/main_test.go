package main

import "testing"

func TestResolveBackendURL(t *testing.T) {
	t.Setenv("AGENT_BACKEND_URL", "")

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "default", want: defaultBackendURL},
		{name: "host and port", input: "127.0.0.1:9090", want: "http://127.0.0.1:9090"},
		{name: "trim slash", input: "https://localhost:8443/", want: "https://localhost:8443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveBackendURL(tt.input)
			if err != nil {
				t.Fatalf("resolveBackendURL() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveBackendURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveBackendURLFromEnvironment(t *testing.T) {
	t.Setenv("AGENT_BACKEND_URL", "127.0.0.1:7777")
	got, err := resolveBackendURL("")
	if err != nil {
		t.Fatalf("resolveBackendURL() error = %v", err)
	}
	if got != "http://127.0.0.1:7777" {
		t.Fatalf("resolveBackendURL() = %q", got)
	}
}

func TestResolveBackendURLRejectsUnsupportedScheme(t *testing.T) {
	t.Setenv("AGENT_BACKEND_URL", "")
	if _, err := resolveBackendURL("file:///tmp/renew"); err == nil {
		t.Fatal("resolveBackendURL() expected an error")
	}
}

func TestJoinRenewURL(t *testing.T) {
	got := joinRenewURL("http://127.0.0.1:8080/base/")
	if got != "http://127.0.0.1:8080/base/renew" {
		t.Fatalf("joinRenewURL() = %q", got)
	}
}

