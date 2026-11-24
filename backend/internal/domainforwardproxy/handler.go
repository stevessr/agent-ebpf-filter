package domainforwardproxy

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
)

type Handler struct {
	settings  DomainForwardProxySettings
	transport http.RoundTripper
	exact     map[string]DomainForwardRoute
	wildcards []DomainForwardRoute
	rewrite   *RewriteKernel
	privacy   *PrivacyEngine
}

var errNoForwardingRoute = errors.New("no forwarding route")

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
		privacy:   NewPrivacyEngine(settings.Privacy),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.settings.Privacy.Enabled && (r.Method == http.MethodConnect ||
		strings.TrimSpace(r.Header.Get("Upgrade")) != "" ||
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")) {
		http.Error(w, "privacy middleware cannot inspect upgraded or tunneled connections", http.StatusNotImplemented)
		return
	}
	host := NormalizeForwardHost(r.Host)
	if host == "" && r.URL != nil {
		host = NormalizeForwardHost(r.URL.Host)
	}
	if host == "" {
		http.Error(w, "missing request host", http.StatusBadRequest)
		return
	}
	if h.settings.Privacy.Enabled && h.settings.AllowAnyHost {
		http.Error(w, "privacy protection requires explicitly allowlisted forward routes", http.StatusUnprocessableEntity)
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

	if h.settings.Privacy.Enabled {
		metadata, checkErr := h.privacy.InspectRequestMetadata(r)
		if checkErr != nil {
			http.Error(w, "privacy metadata inspection failed", http.StatusUnprocessableEntity)
			return
		}
		if metadata.Blocked {
			http.Error(w, "request metadata blocked by privacy policy", http.StatusForbidden)
			return
		}
	}

	if isResponsesWebSocketRequest(r) && h.settings.Privacy.Enabled {
		http.Error(w, "privacy inspection of WebSocket frames is not available; connection refused", http.StatusNotImplemented)
		return
	}
	if isResponsesWebSocketRequest(r) {
		h.serveResponsesWebSocket(w, r, target, host)
		return
	}

	var privacySession *PrivacySession

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

	// The privacy inspection is deliberately the LAST outbound body transform:
	// upstream rewrite/inference must never inject unchecked plaintext.
	if h.settings.Privacy.Enabled && r.Body != nil && r.Body != http.NoBody {
		if !requestBodyIsRewriteable(r) {
			http.Error(w, "request is not inspectable under privacy policy", http.StatusUnprocessableEntity)
			return
		}
		raw, bounded, err := readRequestBodyBounded(r, h.settings.Privacy.MaxBodyBytes)
		if err != nil || !bounded {
			http.Error(w, "request exceeds privacy inspection limit", http.StatusRequestEntityTooLarge)
			return
		}
		encoding := r.Header.Get("Content-Encoding")
		decoded, decodedBounded, err := decodeBodyForRewrite(encoding, raw, h.settings.Privacy.MaxBodyBytes)
		if err != nil || !decodedBounded {
			http.Error(w, "request cannot be decoded for privacy inspection", http.StatusUnprocessableEntity)
			return
		}
		rewritten, session, report, err := h.privacy.ApplyForRecipient(r.Header.Get("Content-Type"), decoded, target.Hostname())
		if err != nil {
			http.Error(w, "privacy inspection failed: request refused", http.StatusUnprocessableEntity)
			return
		}
		if report.Blocked {
			http.Error(w, "request blocked by local privacy policy", http.StatusForbidden)
			return
		}
		privacySession = session
		if report.Redacted {
			encoded, err := encodeBodyAfterRewrite(encoding, rewritten)
			if err != nil {
				http.Error(w, "privacy request encoding failed", http.StatusUnprocessableEntity)
				return
			}
			invalidateRequestBodyIntegrity(r)
			setRequestBody(r, encoded)
		} else {
			restoreRequestBody(r, raw)
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
			if route.UseUpstreamHost {
				out.Host = target.Host
			} else {
				out.Host = r.Host
			}
			out.Header.Set("X-Forwarded-Host", r.Host)
			out.Header.Set("X-Forwarded-Proto", requestForwardedProto(r))
			out.Header.Set("X-Agent-Forward-Route", route.Host)
			// Preserve the client's Accept-Encoding negotiation (gzip, br,
			// zstd) end to end. ModifyResponse now decodes and re-encodes
			// bounded rewriteable bodies while leaving streaming and signed
			// responses untouched.
		},
		Transport: h.transport,
		ModifyResponse: func(response *http.Response) (resultErr error) {
			defer func() {
				if resultErr == nil {
					resultErr = h.restorePrivacyResponse(response, privacySession)
				}
			}()
			if h.rewrite == nil || !h.settings.Rewrite.Enabled || !responseBodyIsRewriteable(r, response) {
				return nil
			}
			contentType := response.Header.Get("Content-Type")
			encoding := response.Header.Get("Content-Encoding")
			if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
				// Keep SSE fully streaming. Compressed SSE is deliberately
				// passed through unchanged; buffering it for rewriting would
				// break incremental event delivery.
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

// restorePrivacyResponse runs after model routing/body rewrites. It does not
// restore anything upstream; placeholder maps exist only for this HTTP request.
func (h *Handler) restorePrivacyResponse(response *http.Response, session *PrivacySession) error {
	if session == nil || !h.settings.Privacy.RestoreResponse || response == nil || response.Body == nil {
		return nil
	}
	encoding := response.Header.Get("Content-Encoding")
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		if normalizeContentEncoding(encoding) != "" {
			return errors.New("cannot restore compressed privacy SSE safely")
		}
		response.Body = newSSEPrivacyBody(response.Body, session, h.settings.Privacy.MaxBodyBytes)
		response.ContentLength = -1
		response.Header.Del("Content-Length")
		response.Header.Del("ETag")
		response.Header.Del("Content-MD5")
		return nil
	}
	raw, bounded, err := readResponseBodyBounded(response, h.settings.Privacy.MaxBodyBytes)
	if err != nil || !bounded {
		return errors.New("privacy response restoration exceeds limit")
	}
	decoded, decodedBounded, err := decodeBodyForRewrite(encoding, raw, h.settings.Privacy.MaxBodyBytes)
	if err != nil || !decodedBounded {
		return errors.New("privacy response restoration decoding failed")
	}
	restored, changed := session.Restore(decoded)
	if !changed {
		restoreResponseBody(response, raw)
		return nil
	}
	encoded, err := encodeBodyAfterRewrite(encoding, restored)
	if err != nil {
		return errors.New("privacy response restoration encoding failed")
	}
	setResponseBody(response, encoded)
	return nil
}
