package domainforwardproxy

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type replayReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *replayReadCloser) Close() error {
	if r == nil || r.closer == nil {
		return nil
	}
	return r.closer.Close()
}

func requestBodyIsRewriteable(r *http.Request) bool {
	if r == nil || r.Body == nil || r.Body == http.NoBody {
		return false
	}
	if strings.TrimSpace(r.Header.Get("Content-Range")) != "" ||
		requestBodyIsSigned(r.Header) ||
		requestBodyIsSigned(r.Trailer) ||
		requestURLIsSigned(r) {
		return false
	}
	return bodyEncodingRewriteable(r.Header.Get("Content-Encoding"))
}

func requestBodyIsSigned(header http.Header) bool {
	if header == nil {
		return false
	}
	for _, name := range []string{
		"Signature",
		"Signature-Input",
		"X-Amz-Content-Sha256",
		"X-Goog-Content-Sha256",
	} {
		if headerHasFieldFold(header, name) {
			return true
		}
	}
	authorization := strings.ToUpper(strings.TrimSpace(header.Get("Authorization")))
	for _, scheme := range []string{
		"AWS4-HMAC-SHA256 ",
		"SIGNATURE ",
		"SHAREDKEY ",
		"SHAREDKEYLITE ",
	} {
		if strings.HasPrefix(authorization, scheme) {
			return true
		}
	}
	return false
}

func requestURLIsSigned(request *http.Request) bool {
	if request == nil || request.URL == nil {
		return false
	}
	query := request.URL.Query()
	switch {
	case queryHasFold(query, "X-Amz-Signature") && queryHasFold(query, "X-Amz-Algorithm"):
		return true
	case queryHasFold(query, "X-Goog-Signature") && queryHasFold(query, "X-Goog-Algorithm"):
		return true
	case queryHasFold(query, "sig") && queryHasFold(query, "sv"):
		return true
	case queryHasFold(query, "Signature") && queryHasFold(query, "Key-Pair-Id"):
		return true
	default:
		return false
	}
}

func queryHasFold(query url.Values, name string) bool {
	for key := range query {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func responseBodyIsRewriteable(request *http.Request, response *http.Response) bool {
	if response == nil || response.Body == nil || response.Body == http.NoBody {
		return false
	}
	if request != nil && request.Method == http.MethodHead {
		return false
	}
	if response.StatusCode >= 100 && response.StatusCode < 200 ||
		response.StatusCode == http.StatusNoContent ||
		response.StatusCode == http.StatusNotModified ||
		response.StatusCode == http.StatusPartialContent ||
		strings.TrimSpace(response.Header.Get("Content-Range")) != "" ||
		messageSignatureFieldsPresent(response.Header) ||
		messageSignatureFieldsPresent(response.Trailer) ||
		strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "multipart/byteranges") {
		return false
	}
	return bodyEncodingRewriteable(response.Header.Get("Content-Encoding"))
}

func readRequestBodyBounded(r *http.Request, limit int64) ([]byte, bool, error) {
	if r.ContentLength > limit && r.ContentLength >= 0 {
		return nil, false, nil
	}
	prefix, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(prefix)) > limit {
		original := r.Body
		r.Body = &replayReadCloser{
			Reader: io.MultiReader(bytes.NewReader(prefix), original),
			closer: original,
		}
		return nil, false, nil
	}
	_ = r.Body.Close()
	return prefix, true, nil
}

func readResponseBodyBounded(response *http.Response, limit int64) ([]byte, bool, error) {
	if response.ContentLength > limit && response.ContentLength >= 0 {
		return nil, false, nil
	}
	prefix, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(prefix)) > limit {
		original := response.Body
		response.Body = &replayReadCloser{
			Reader: io.MultiReader(bytes.NewReader(prefix), original),
			closer: original,
		}
		return nil, false, nil
	}
	_ = response.Body.Close()
	return prefix, true, nil
}

func restoreRequestBody(r *http.Request, body []byte) {
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
}

func setRequestBody(r *http.Request, body []byte) {
	restoreRequestBody(r, body)
	r.TransferEncoding = nil
	r.Header.Del("Transfer-Encoding")
	if len(r.Trailer) > 0 {
		// Trailers need end-of-body framing. Leave the final transfer framing
		// to net/http so HTTP/1.1 can use chunked encoding while HTTP/2+ keeps
		// protocol-native trailers.
		r.ContentLength = -1
		r.Header.Del("Content-Length")
		return
	}
	r.ContentLength = int64(len(body))
	r.Header.Set("Content-Length", strconv.Itoa(len(body)))
}

func restoreResponseBody(response *http.Response, body []byte) {
	response.Body = io.NopCloser(bytes.NewReader(body))
	if len(response.Trailer) > 0 {
		// Preserve trailer-capable framing. HTTP/1.1 needs chunked transfer,
		// while HTTP/2+ carries trailers in the protocol without chunking.
		response.ContentLength = -1
		response.Header.Del("Content-Length")
		return
	}
	response.ContentLength = int64(len(body))
	response.TransferEncoding = nil
	response.Header.Set("Content-Length", strconv.Itoa(len(body)))
	response.Header.Del("Transfer-Encoding")
}

func setResponseBody(response *http.Response, body []byte) {
	invalidateResponseBodyIntegrity(response)
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.TransferEncoding = nil
	response.Header.Del("Transfer-Encoding")
	if len(response.Trailer) > 0 {
		response.ContentLength = -1
		response.Header.Del("Content-Length")
		return
	}
	response.ContentLength = int64(len(body))
	response.Header.Set("Content-Length", strconv.Itoa(len(body)))
}

func invalidateRequestBodyIntegrity(request *http.Request) {
	if request == nil {
		return
	}
	for _, name := range []string{
		"Content-MD5",
		"Digest",
		"Content-Digest",
		"Repr-Digest",
	} {
		deleteHeaderFold(request.Header, name)
		deleteHeaderFold(request.Trailer, name)
	}
}

func invalidateResponseBodyIntegrity(response *http.Response) {
	if response == nil {
		return
	}
	for _, name := range []string{
		"ETag",
		"Content-MD5",
		"Digest",
		"Content-Digest",
		"Repr-Digest",
	} {
		deleteHeaderFold(response.Header, name)
		deleteHeaderFold(response.Trailer, name)
	}
}

func messageSignatureFieldsPresent(header http.Header) bool {
	return headerHasFieldFold(header, "Signature") ||
		headerHasFieldFold(header, "Signature-Input")
}

func headerHasFieldFold(header http.Header, name string) bool {
	for key := range header {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func deleteHeaderFold(header http.Header, name string) {
	for key := range header {
		if strings.EqualFold(key, name) {
			delete(header, key)
		}
	}
}
