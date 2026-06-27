package tls

import (
	"testing"
)

func TestDetectSSLDataType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"http_request", "GET /api/v1/users HTTP/1.1\r\nHost: example.com", "http_request"},
		{"http_response", "HTTP/1.1 200 OK\r\nContent-Type: application/json", "http_response"},
		{"post_request", "POST /graphql HTTP/1.1\r\nHost: api.example.com", "http_request"},
		{"sse_data", "data: {\"type\":\"message\"}\n\n", "sse"},
		{"sse_event", "event: update\ndata: payload\n\n", "sse"},
		{"json_object", `{"key": "value", "nested": {"a": 1}}`, "json"},
		{"json_array", `[{"id": 1}, {"id": 2}]`, "json"},
		{"empty", "", "empty"},
		{"text_plain", "Hello, World!", "text"},
		{"grpc", string([]byte{0, 0, 0, 0, 0x80}), "grpc"},
		{"binary_null", string([]byte{0x00, 0x01, 0x02, 0x03, 0x04}), "binary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectSSLDataType(tt.input)
			if result != tt.expected {
				t.Errorf("DetectSSLDataType(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseSSLFilterExpr(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		data     map[string]any
		expected bool
	}{
		{
			name:     "exact_match",
			expr:     "function=READ/RECV",
			data:     map[string]any{"function": "READ/RECV"},
			expected: true,
		},
		{
			name:     "exact_no_match",
			expr:     "function=READ/RECV",
			data:     map[string]any{"function": "WRITE/SEND"},
			expected: false,
		},
		{
			name:     "contains_match",
			expr:     "data~chunked",
			data:     map[string]any{"data": "chunked transfer encoding"},
			expected: true,
		},
		{
			name:     "gt_match",
			expr:     "len>10",
			data:     map[string]any{"len": uint64(100)},
			expected: true,
		},
		{
			name:     "gt_no_match",
			expr:     "len>10",
			data:     map[string]any{"len": uint64(5)},
			expected: false,
		},
		{
			name:     "and_match",
			expr:     "data~chunked&function=READ/RECV",
			data:     map[string]any{"data": "chunked", "function": "READ/RECV"},
			expected: true,
		},
		{
			name:     "and_no_match",
			expr:     "data~chunked&function=READ/RECV",
			data:     map[string]any{"data": "chunked", "function": "WRITE/SEND"},
			expected: false,
		},
		{
			name:     "or_match",
			expr:     "data~chunked|function=READ/RECV",
			data:     map[string]any{"data": "plain", "function": "READ/RECV"},
			expected: true,
		},
		{
			name:     "data_type_match",
			expr:     "data_type=http_request",
			data:     map[string]any{"data": "GET / HTTP/1.1"},
			expected: true,
		},
		{
			name:     "is_handshake_true",
			expr:     "is_handshake=true",
			data:     map[string]any{"is_handshake": true},
			expected: true,
		},
		{
			name:     "truncated_false",
			expr:     "truncated=true",
			data:     map[string]any{"truncated": false},
			expected: false,
		},
		{
			name:     "not_equal",
			expr:     "direction!=SEND",
			data:     map[string]any{"direction": "RECV"},
			expected: true,
		},
		{
			name:     "empty_expr",
			expr:     "",
			data:     map[string]any{},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := ParseSSLFilterExpr(tt.expr)
			result := filter.Evaluate(tt.data)
			if result != tt.expected {
				t.Errorf("ParseSSLFilterExpr(%q).Evaluate(%v) = %v, want %v",
					tt.expr, tt.data, result, tt.expected)
			}
		})
	}
}

func TestProcessEscapeSequences(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`0\r\n\r\n`, "0\r\n\r\n"},
		{`hello\tworld\n`, "hello\tworld\n"},
		{`path\\to\\file`, `path\to\file`},
		{`escaped\"quote`, `escaped"quote`},
	}
	for _, tt := range tests {
		result := processEscapeSequences(tt.input)
		if result != tt.expected {
			t.Errorf("processEscapeSequences(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestIsTLSHandshakeFragment(t *testing.T) {
	// TLS handshake content type = 0x16
	handshake := CompletedTLSFragment{Payload: []byte{0x16, 0x03, 0x03, 0x00, 0x10}}
	if !isTLSHandshakeFragment(handshake) {
		t.Error("expected handshake fragment to be detected")
	}
	// Application data content type = 0x17
	appData := CompletedTLSFragment{Payload: []byte{0x17, 0x03, 0x03, 0x00, 0x10}}
	if isTLSHandshakeFragment(appData) {
		t.Error("expected app data fragment to NOT be handshake")
	}
	// Empty payload
	empty := CompletedTLSFragment{Payload: []byte{}}
	if isTLSHandshakeFragment(empty) {
		t.Error("expected empty fragment to NOT be handshake")
	}
}
