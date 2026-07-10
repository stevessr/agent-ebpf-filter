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
		},
		Transport: h.transport,
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
