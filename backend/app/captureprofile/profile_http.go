package captureprofile

import (
	"net/url"
	"strconv"
	"strings"
)

func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if strings.Contains(host, "://") {
		if parsed, err := url.Parse(host); err == nil {
			host = parsed.Host
		}
	}
	if colon := strings.LastIndex(host, ":"); colon > 0 && !strings.Contains(host[colon+1:], "]") {
		if strings.Count(host, ":") == 1 {
			host = host[:colon]
		}
	}
	return strings.TrimSuffix(strings.Trim(host, "[]"), ".")
}

// RequestPath drops query/fragment data before matching or persistence. API
// keys are frequently placed in query strings, while provider fingerprints are
// generally stable on the path component.
func RequestPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// Capture parsers normally hand us a path that has already been separated
	// from its authority. Avoid net/url.Parse on this overwhelmingly common hot
	// path: it allocates even for simple strings such as "/v1/jobs/42". Only
	// absolute URLs and authority-form references need URL parsing.
	needsURLParse := strings.HasPrefix(raw, "//") || strings.Contains(raw, "://")
	if !needsURLParse {
		if index := strings.IndexAny(raw, "?#"); index >= 0 {
			raw = raw[:index]
		}
		return raw
	}

	if parsed, err := url.Parse(raw); err == nil {
		if parsed.Path != "" {
			return parsed.EscapedPath()
		}
	}
	if index := strings.IndexAny(raw, "?#"); index >= 0 {
		raw = raw[:index]
	}
	return raw
}

type HTTP1StartLine struct {
	Kind   string
	Method string
	Path   string
	Status uint32
}

func ParseHTTP1StartLine(line string) (HTTP1StartLine, bool) {
	line = strings.TrimSpace(strings.TrimRight(line, "\x00"))
	if line == "" {
		return HTTP1StartLine{}, false
	}
	if index := strings.IndexAny(line, "\r\n"); index >= 0 {
		line = line[:index]
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return HTTP1StartLine{}, false
	}
	if strings.HasPrefix(strings.ToUpper(parts[0]), "HTTP/1.") {
		status, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil || status < 100 || status > 999 {
			return HTTP1StartLine{}, false
		}
		return HTTP1StartLine{Kind: "response", Status: uint32(status)}, true
	}
	method := strings.ToUpper(parts[0])
	if !isHTTPMethod(method) {
		return HTTP1StartLine{}, false
	}
	path := RequestPath(parts[1])
	if path == "" {
		return HTTP1StartLine{}, false
	}
	return HTTP1StartLine{Kind: "request", Method: method, Path: path}, true
}

func ParseHTTP1RequestLine(line string) (method, path string, ok bool) {
	parsed, ok := ParseHTTP1StartLine(line)
	if !ok || parsed.Kind != "request" {
		return "", "", false
	}
	return parsed.Method, parsed.Path, true
}

// ParseGRPCPath normalizes the canonical HTTP/2 gRPC route
// /package.Service/Method without inspecting protobuf message bodies.
func ParseGRPCPath(raw string) (service, method string, ok bool) {
	path := strings.TrimPrefix(RequestPath(raw), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	service = strings.TrimSpace(parts[0])
	method = strings.TrimSpace(parts[1])
	if service == "" || method == "" {
		return "", "", false
	}
	return service, method, true
}

func isHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE", "CONNECT":
		return true
	default:
		return false
	}
}

func hasHeader(headers map[string]string, wanted string) bool {
	for key := range headers {
		if strings.EqualFold(strings.TrimSpace(key), wanted) {
			return true
		}
	}
	return false
}
