package domainforwardproxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type Handler struct {
	settings  DomainForwardProxySettings
	transport http.RoundTripper
	exact     map[string]DomainForwardRoute
	wildcards []DomainForwardRoute
	rewrite   *RewriteKernel
}

var errNoForwardingRoute = errors.New("no forwarding route")

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

func NewHandler(settings DomainForwardProxySettings) *Handler {
	return NewHandlerWithTransport(settings, nil)
}

// NewHandlerWithTransport creates a proxy handler with an optional custom
// transport. A nil transport keeps the production DNS, proxy, and timeout
// behavior; a non-nil transport replaces those policies and is intended for
// controlled embedding and tests.
func NewHandlerWithTransport(settings DomainForwardProxySettings, transport http.RoundTripper) *Handler {
	NormalizeSettings(&settings)
	exact := make(map[string]DomainForwardRoute, len(settings.Routes))
	wildcards := make([]DomainForwardRoute, 0)
	for _, route := range settings.Routes {
		pattern := NormalizeDomainPattern(route.Host)
		if pattern == "" {
			continue
		}
		route.Host = pattern
		if strings.HasPrefix(pattern, "*.") {
			wildcards = append(wildcards, route)
			continue
		}
		exact[pattern] = route
	}
	sort.SliceStable(wildcards, func(i, j int) bool {
		return len(wildcards[i].Host) > len(wildcards[j].Host)
	})
	if transport == nil {
		transport = NewTransport(settings)
	}
	return &Handler{
		settings:  settings,
		transport: transport,
		exact:     exact,
		wildcards: wildcards,
		rewrite:   NewRewriteKernel(settings.Rewrite),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := NormalizeForwardHost(r.Host)
	if host == "" && r.URL != nil {
		host = NormalizeForwardHost(r.URL.Host)
	}
	if host == "" {
		http.Error(w, "missing request host", http.StatusBadRequest)
		return
	}
	target, route, err := h.TargetForHost(host)
	if err != nil {
		if errors.Is(err, errNoForwardingRoute) {
			http.Error(w, "no forwarding route for requested host", http.StatusBadGateway)
			return
		}
		log.Printf("[DOMAIN-FORWARD] route resolution for host %s failed: %v", host, err)
		http.Error(w, "forwarding route is invalid", http.StatusBadGateway)
		return
	}

	if isResponsesWebSocketRequest(r) {
		h.serveResponsesWebSocket(w, r, target, host)
		return
	}

	var modelMapping *modelRewrite
	if h.rewrite != nil && h.settings.Rewrite.Enabled && requestBodyIsRewriteable(r) {
		rawBody, bounded, err := readRequestBodyBounded(r, h.rewrite.MaxBodyBytes())
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if bounded {
			encoding := r.Header.Get("Content-Encoding")
			body, decodedBounded, err := decodeBodyForRewrite(encoding, rawBody, h.rewrite.MaxBodyBytes())
			if err != nil {
				http.Error(w, "invalid request body encoding", http.StatusBadRequest)
				return
			}
			if !decodedBounded {
				restoreRequestBody(r, rawBody)
			} else {
				rewritten, mapping, changed := h.rewrite.RewriteRequest(host, r.URL.Path, r.Header.Get("Content-Type"), body)
				modelMapping = mapping
				if changed {
					encoded, err := encodeBodyAfterRewrite(encoding, rewritten)
					if err != nil {
						http.Error(w, "failed to encode rewritten request body", http.StatusBadRequest)
						return
					}
					invalidateRequestBodyIntegrity(r)
					setRequestBody(r, encoded)
				} else {
					restoreRequestBody(r, rawBody)
				}
			}
		}
	}

	proxy := &httputil.ReverseProxy{
		Director: func(out *http.Request) {
			out.URL.Scheme = target.Scheme
			out.URL.Host = target.Host
			out.URL.Path, out.URL.RawPath = JoinPath(target, r.URL)
			if target.RawQuery == "" || out.URL.RawQuery == "" {
				out.URL.RawQuery = target.RawQuery + out.URL.RawQuery
			} else {
				out.URL.RawQuery = target.RawQuery + "&" + out.URL.RawQuery
			}
			out.Host = r.Host
			out.Header.Set("X-Forwarded-Host", r.Host)
			out.Header.Set("X-Forwarded-Proto", requestForwardedProto(r))
			out.Header.Set("X-Agent-Forward-Route", route.Host)
			if h.rewrite != nil && h.settings.Rewrite.Enabled {
				// Ask upstreams for identity encoding so response body rules do
				// not silently stop applying when the client advertised gzip.
				out.Header.Del("Accept-Encoding")
			}
		},
		Transport: h.transport,
		ModifyResponse: func(response *http.Response) error {
			if h.rewrite == nil || !h.settings.Rewrite.Enabled || !responseBodyIsRewriteable(r, response) {
				return nil
			}
			contentType := response.Header.Get("Content-Type")
			encoding := response.Header.Get("Content-Encoding")
			if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
				// Keep SSE fully streaming. Compressed SSE is deliberately left
				// untouched; the production transport normally negotiates or
				// auto-decompresses gzip before ModifyResponse.
				if normalizeContentEncoding(encoding) != "" {
					return nil
				}
				response.Body = newSSERewriteBody(response.Body, h.rewrite, host, r.URL.Path, contentType, modelMapping)
				response.ContentLength = -1
				response.Header.Del("Content-Length")
				return nil
			}
			rawBody, bounded, err := readResponseBodyBounded(response, h.rewrite.MaxBodyBytes())
			if err != nil || !bounded {
				return err
			}
			body, decodedBounded, err := decodeBodyForRewrite(encoding, rawBody, h.rewrite.MaxBodyBytes())
			if err != nil {
				return err
			}
			if !decodedBounded {
				restoreResponseBody(response, rawBody)
				return nil
			}
			rewritten, changed := h.rewrite.RewriteResponse(host, r.URL.Path, contentType, body, modelMapping)
			if !changed {
				restoreResponseBody(response, rawBody)
				return nil
			}
			encoded, err := encodeBodyAfterRewrite(encoding, rewritten)
			if err != nil {
				return err
			}
			setResponseBody(response, encoded)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			log.Printf("[DOMAIN-FORWARD] upstream %s for host %s failed: %v", target.String(), host, err)
			http.Error(w, "upstream request failed", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
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

func (h *Handler) TargetForHost(host string) (*url.URL, DomainForwardRoute, error) {
	host = NormalizeForwardHost(host)
	if host == "" {
		return nil, DomainForwardRoute{}, errors.New("empty host")
	}
	if route, ok := h.lookupRoute(host); ok {
		target, err := ParseUpstream(route.Upstream, h.settings.DefaultScheme, host)
		return target, route, err
	}
	if h.settings.AllowAnyHost {
		route := DomainForwardRoute{Host: host}
		target, err := ParseUpstream("", h.settings.DefaultScheme, host)
		return target, route, err
	}
	return nil, DomainForwardRoute{}, fmt.Errorf("%w for host %q", errNoForwardingRoute, host)
}

func (h *Handler) lookupRoute(host string) (DomainForwardRoute, bool) {
	if route, ok := h.exact[host]; ok {
		return route, true
	}
	for _, route := range h.wildcards {
		suffix := strings.TrimPrefix(route.Host, "*.")
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return route, true
		}
	}
	return DomainForwardRoute{}, false
}

func requestForwardedProto(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func ParseUpstream(raw string, defaultScheme string, requestHost string) (*url.URL, error) {
	defaultScheme = normalizedForwardScheme(defaultScheme)
	upstream := strings.TrimSpace(raw)
	if upstream == "" {
		upstream = defaultScheme + "://" + requestHost
	} else {
		upstream = strings.ReplaceAll(upstream, "{host}", requestHost)
		if !strings.Contains(upstream, "://") {
			upstream = defaultScheme + "://" + upstream
		}
	}
	target, err := url.Parse(upstream)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream %q: %w", raw, err)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf("unsupported upstream scheme %q", target.Scheme)
	}
	if strings.TrimSpace(target.Host) == "" {
		return nil, fmt.Errorf("upstream %q does not include a host", upstream)
	}
	return target, nil
}

func JoinPath(target *url.URL, incoming *url.URL) (path, rawpath string) {
	if target == nil || incoming == nil {
		return "", ""
	}
	targetPath := target.EscapedPath()
	incomingPath := incoming.EscapedPath()
	if targetPath == "" {
		return incoming.Path, incoming.RawPath
	}
	joined := singleJoiningSlash(targetPath, incomingPath)
	if target.RawPath == "" && incoming.RawPath == "" {
		return singleJoiningSlash(target.Path, incoming.Path), ""
	}
	return joined, joined
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
}
