package domainforwardproxy

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type trackingReadCloser struct {
	reader *strings.Reader
	closed bool
}

func newTrackingReadCloser(value string) *trackingReadCloser {
	return &trackingReadCloser{reader: strings.NewReader(value)}
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestReadRequestBodyBoundedPreservesOriginalCloser(t *testing.T) {
	const payload = "0123456789"
	original := newTrackingReadCloser(payload)
	request := &http.Request{
		Body:          original,
		ContentLength: -1,
		Header:        make(http.Header),
	}

	body, bounded, err := readRequestBodyBounded(request, 4)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	if bounded || body != nil {
		t.Fatalf("expected over-limit passthrough, bounded=%v body=%q", bounded, body)
	}
	got, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read reconstructed request body: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("reconstructed request body = %q, want %q", got, payload)
	}
	if original.closed {
		t.Fatal("original request body closed before proxy consumed it")
	}
	if err := request.Body.Close(); err != nil {
		t.Fatalf("close request body: %v", err)
	}
	if !original.closed {
		t.Fatal("request body wrapper did not close original stream")
	}
}

func TestReadResponseBodyBoundedPreservesOriginalCloser(t *testing.T) {
	const payload = "abcdefghij"
	original := newTrackingReadCloser(payload)
	response := &http.Response{
		Body:          original,
		ContentLength: -1,
		Header:        make(http.Header),
	}

	body, bounded, err := readResponseBodyBounded(response, 4)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if bounded || body != nil {
		t.Fatalf("expected over-limit passthrough, bounded=%v body=%q", bounded, body)
	}
	got, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read reconstructed response body: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("reconstructed response body = %q, want %q", got, payload)
	}
	if original.closed {
		t.Fatal("original response body closed before downstream consumed it")
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
	if !original.closed {
		t.Fatal("response body wrapper did not close original stream")
	}
}
