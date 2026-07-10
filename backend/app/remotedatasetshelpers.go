package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section remotedatasetshelpers.go ----

const remoteDatasetRedirectLimit = 5

type remoteDatasetResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type remoteDatasetDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

var remoteDatasetBlockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("168.63.129.16/32"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fec0::/10"),
}

var defaultRemoteDatasetHTTPClient = newRemoteDatasetHTTPClient(net.DefaultResolver, &net.Dialer{
	Timeout:   10 * time.Second,
	KeepAlive: 30 * time.Second,
})

func newRemoteDatasetHTTPClient(resolver remoteDatasetResolver, dialer remoteDatasetDialer) *http.Client {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = newRemoteDatasetDialContext(resolver, dialer)
	transport.DialTLSContext = nil
	return &http.Client{
		Timeout:       20 * time.Second,
		Transport:     transport,
		CheckRedirect: checkRemoteDatasetRedirect,
	}
}

func newRemoteDatasetDialContext(resolver remoteDatasetResolver, dialer remoteDatasetDialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid remote dataset address: %w", err)
		}
		host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
		if host == "" || strings.Contains(host, "%") {
			return nil, errors.New("remote dataset host is invalid")
		}

		addresses := make([]netip.Addr, 0, 4)
		if literal, parseErr := netip.ParseAddr(host); parseErr == nil {
			addresses = append(addresses, literal.Unmap())
		} else {
			resolved, resolveErr := resolver.LookupNetIP(ctx, "ip", host)
			if resolveErr != nil {
				return nil, fmt.Errorf("resolve remote dataset host %q: %w", host, resolveErr)
			}
			addresses = append(addresses, resolved...)
		}
		if len(addresses) == 0 {
			return nil, fmt.Errorf("remote dataset host %q did not resolve to an address", host)
		}

		seen := make(map[netip.Addr]struct{}, len(addresses))
		candidates := make([]netip.Addr, 0, len(addresses))
		for _, address := range addresses {
			address = address.Unmap()
			if !isPublicRemoteDatasetAddress(address) {
				return nil, fmt.Errorf("remote dataset host %q resolves to a non-public address", host)
			}
			if _, ok := seen[address]; ok {
				continue
			}
			seen[address] = struct{}{}
			candidates = append(candidates, address)
		}

		var lastErr error
		for _, candidate := range candidates {
			if network == "tcp4" && !candidate.Is4() {
				continue
			}
			if network == "tcp6" && !candidate.Is6() {
				continue
			}
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		if lastErr != nil {
			return nil, fmt.Errorf("dial remote dataset host %q: %w", host, lastErr)
		}
		return nil, fmt.Errorf("remote dataset host %q has no address compatible with %s", host, network)
	}
}

func isPublicRemoteDatasetAddress(address netip.Addr) bool {
	if !address.IsValid() || address.Zone() != "" {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, prefix := range remoteDatasetBlockedPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func checkRemoteDatasetRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= remoteDatasetRedirectLimit {
		return fmt.Errorf("stopped after %d remote dataset redirects", remoteDatasetRedirectLimit)
	}
	if req == nil || req.URL == nil {
		return errors.New("remote dataset redirect URL is missing")
	}
	if _, err := validateRemoteDatasetURL(req.URL.String()); err != nil {
		return fmt.Errorf("invalid remote dataset redirect: %w", err)
	}
	if len(via) > 0 && via[len(via)-1] != nil && via[len(via)-1].URL != nil && strings.EqualFold(via[len(via)-1].URL.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
		return errors.New("remote dataset redirect cannot downgrade HTTPS to HTTP")
	}
	return nil
}

func downloadRemoteDatasetWithClient(rawURL string, client *http.Client) ([]byte, string, error) {
	parsed, err := validateRemoteDatasetURL(rawURL)
	if err != nil {
		return nil, "", err
	}

	if client == nil {
		client = defaultRemoteDatasetHTTPClient
	}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "agent-ebpf-filter/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("remote dataset fetch failed: %s", resp.Status)
	}
	if resp.ContentLength > remoteDatasetFetchLimitBytes {
		return nil, "", fmt.Errorf("remote dataset is larger than %d bytes", remoteDatasetFetchLimitBytes)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, remoteDatasetFetchLimitBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > remoteDatasetFetchLimitBytes {
		return nil, "", fmt.Errorf("remote dataset is larger than %d bytes", remoteDatasetFetchLimitBytes)
	}

	contentType := resp.Header.Get("Content-Type")
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
		contentType = mediaType
	}
	return body, contentType, nil
}

func validateRemoteDatasetURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	parsed.Scheme = strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported dataset URL scheme: %s", parsed.Scheme)
	}
	if parsed.Opaque != "" || parsed.Host == "" || strings.TrimSpace(parsed.Hostname()) == "" {
		return nil, errors.New("dataset URL must include a host")
	}
	if parsed.User != nil {
		return nil, errors.New("dataset URL must not include user credentials")
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(parsed.Hostname())), ".")
	if strings.Contains(host, "%") {
		return nil, errors.New("dataset URL must not include an IPv6 zone")
	}
	if port := parsed.Port(); port != "" {
		value, portErr := strconv.Atoi(port)
		if portErr != nil || value < 1 || value > 65535 {
			return nil, errors.New("dataset URL includes an invalid port")
		}
	}
	if literal, parseErr := netip.ParseAddr(host); parseErr == nil && !isPublicRemoteDatasetAddress(literal) {
		return nil, errors.New("dataset URL must not target a non-public address")
	}
	return parsed, nil
}

func normalizeRemoteDatasetFormat(format, sourceURL string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format != "" && format != "auto" {
		return format
	}

	ext := strings.ToLower(filepath.Ext(sourceURL))
	switch ext {
	case ".json":
		return "json"
	case ".jsonl", ".ndjson":
		return "jsonl"
	case ".csv":
		return "csv"
	case ".tsv":
		return "tsv"
	case ".txt", ".log":
		return "text"
	default:
		return "auto"
	}
}

func normalizeRemoteDatasetLabelMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "preserve", "keep", "source":
		return "preserve"
	case "unlabeled", "manual", "none":
		return "unlabeled"
	case "heuristic", "auto", "automatic":
		return "heuristic"
	case "block", "dangerous", "highrisk", "high-risk":
		return "block"
	default:
		return "preserve"
	}
}

func contentTypeForDatasetFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return "application/json"
	case "jsonl", "ndjson":
		return "application/x-ndjson"
	case "csv":
		return "text/csv; charset=utf-8"
	case "tsv":
		return "text/tab-separated-values; charset=utf-8"
	case "text", "txt":
		return "text/plain; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

func parseDatasetLimit(limit int) int {
	if limit <= 0 {
		return 200
	}
	if limit > 5000 {
		return 5000
	}
	return limit
}

func trainingSampleToRemoteDatasetRow(index int, sample TrainingSample) remoteDatasetRow {
	label := sampleLabelName(sample.Label)
	if label == "" {
		label = "-"
	}
	timestamp := sample.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	return remoteDatasetRow{
		Row:          index,
		CommandLine:  trainingSampleCommandLine(sample),
		Comm:         sample.Comm,
		Args:         append([]string(nil), sample.Args...),
		Label:        label,
		LabelSource:  sample.UserLabel,
		Category:     sample.Category,
		AnomalyScore: sample.AnomalyScore,
		HasAnomaly:   true,
		Timestamp:    timestamp.Format(time.RFC3339),
		UserLabel:    sample.UserLabel,
	}
}
