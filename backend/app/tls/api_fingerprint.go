package tls

import (
	"strings"

	"agent-ebpf-filter/app/captureprofile"
)

func tlsCaptureProtocol(event *TLSPlaintextEvent) string {
	if event == nil {
		return ""
	}
	if strings.HasPrefix(event.Type, "http2_") {
		contentType := strings.ToLower(strings.TrimSpace(event.ContentType))
		if contentType == "" {
			for key, value := range event.Headers {
				if strings.EqualFold(strings.TrimSpace(key), "content-type") {
					contentType = strings.ToLower(strings.TrimSpace(value))
					break
				}
			}
		}
		if strings.Contains(contentType, "application/grpc") {
			return "grpc"
		}
		return "http2"
	}
	if event.Type == "http_request" || event.Type == "http_response" {
		return "http1"
	}
	return ""
}

// annotateTLSAPIFingerprint turns the protocol parser output into a common API
// capture identity. The matcher only sees sanitized metadata; bodies and secret
// query parameters are never part of the fingerprint input.
func annotateTLSAPIFingerprint(event *TLSPlaintextEvent) {
	if event == nil {
		return
	}
	protocol := tlsCaptureProtocol(event)
	if protocol == "" {
		return
	}
	event.CaptureSource = "tls_plaintext"
	event.AppProtocol = protocol
	event.RequestPath = captureprofile.RequestPath(event.URL)

	match, ok := captureprofile.Default.Match(captureprofile.Observation{
		Source:      event.CaptureSource,
		Protocol:    protocol,
		Direction:   event.Direction,
		Method:      event.Method,
		Host:        event.Host,
		Path:        event.RequestPath,
		Headers:     event.Headers,
		ContentType: event.ContentType,
		Transport:   "tcp",
		Process:     event.Comm,
	})
	if ok {
		event.APIProfile = match.ProfileID
		event.APIProduct = match.Product
		event.APIOperation = match.Operation
		event.APIConfidence = match.Confidence
		if event.Vendor == "" || !strings.HasSuffix(match.Vendor, "-compatible") {
			event.Vendor = match.Vendor
		}
		return
	}
	if protocol == "grpc" {
		if service, method, parsed := captureprofile.ParseGRPCPath(event.RequestPath); parsed {
			event.APIProfile = "grpc:" + service
			event.APIProduct = service
			event.APIOperation = method
			event.APIConfidence = 70
		}
	}
}
