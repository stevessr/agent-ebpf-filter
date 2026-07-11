package app

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend_test.go section remotedatasets_test.go ----

type staticRemoteDatasetResolver struct {
	addresses map[string][]netip.Addr
	err       error
	calls     []string
}

type sequencedRemoteDatasetResolver struct {
	responses [][]netip.Addr
	calls     int
}

func (resolver *sequencedRemoteDatasetResolver) LookupNetIP(_ context.Context, _ string, _ string) ([]netip.Addr, error) {
	if resolver.calls >= len(resolver.responses) {
		return nil, nil
	}
	response := append([]netip.Addr(nil), resolver.responses[resolver.calls]...)
	resolver.calls++
	return response, nil
}

func (resolver *staticRemoteDatasetResolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	resolver.calls = append(resolver.calls, host)
	if resolver.err != nil {
		return nil, resolver.err
	}
	return append([]netip.Addr(nil), resolver.addresses[host]...), nil
}

type recordingRemoteDatasetDialer struct {
	addresses []string
	err       error
}

func (dialer *recordingRemoteDatasetDialer) DialContext(_ context.Context, _ string, address string) (net.Conn, error) {
	dialer.addresses = append(dialer.addresses, address)
	return nil, dialer.err
}

func TestParseRemoteDatasetRecordsJSONL(t *testing.T) {
	raw := []byte(`{"commandLine":"rm -rf /tmp/demo","label":"BLOCK"}
{"commandLine":"echo hello"}
`)

	records, format, err := parseRemoteDatasetRecords(raw, "auto", "inline.jsonl")
	if err != nil {
		t.Fatalf("parseRemoteDatasetRecords() error = %v", err)
	}
	if format != "jsonl" {
		t.Fatalf("format = %q, want jsonl", format)
	}
	if len(records) != 2 {
		t.Fatalf("records length = %d, want 2", len(records))
	}
	if records[0].Comm != "rm" || records[0].Label != "BLOCK" {
		t.Fatalf("first record = %#v", records[0])
	}
	if records[1].Comm != "echo" || strings.Join(records[1].Args, " ") != "hello" {
		t.Fatalf("second record = %#v", records[1])
	}
}

func TestParseRemoteDatasetRecordsWithLimitsAcrossFormats(t *testing.T) {
	tests := []struct {
		name   string
		format string
		raw    string
	}{
		{
			name:   "json",
			format: "json",
			raw:    `[{"commandLine":"echo 1"},{"commandLine":"echo 2"},{"commandLine":"echo 3"},{"commandLine":"echo 4"}]`,
		},
		{
			name:   "jsonl",
			format: "jsonl",
			raw:    "{\"commandLine\":\"echo 1\"}\n{\"commandLine\":\"echo 2\"}\n{\"commandLine\":\"echo 3\"}\n{\"commandLine\":\"echo 4\"}\n",
		},
		{
			name:   "csv",
			format: "csv",
			raw:    "commandLine\necho 1\necho 2\necho 3\necho 4\n",
		},
		{
			name:   "tsv",
			format: "tsv",
			raw:    "commandLine\tlabel\necho 1\tALLOW\necho 2\tALLOW\necho 3\tALLOW\necho 4\tALLOW\n",
		},
		{
			name:   "text",
			format: "text",
			raw:    "echo 1\necho 2\necho 3\necho 4\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseRemoteDatasetRecordsWithLimits([]byte(tt.raw), tt.format, tt.name, remoteDatasetParseLimits{MaxRecords: 3, StoreRecords: 2})
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if result.Total != 3 || len(result.Records) != 2 || !result.Truncated {
				t.Fatalf("result total/stored/truncated = %d/%d/%t, want 3/2/true", result.Total, len(result.Records), result.Truncated)
			}
			if result.Records[0].CommandLine != "echo 1" || result.Records[1].CommandLine != "echo 2" {
				t.Fatalf("stored records = %#v", result.Records)
			}
		})
	}
}

func TestParseRemoteDatasetLimitedJSONIsDeterministic(t *testing.T) {
	raw := []byte(`{
		"executables": {
			"zeta": {"functions":{"shell":[{"code":"zeta --shell"}]}},
			"alpha": {"functions":{"shell":[{"code":"alpha --shell"}]}},
			"middle": {"functions":{"shell":[{"code":"middle --shell"}]}}
		}
	}`)

	for i := 0; i < 10; i++ {
		result, err := parseRemoteDatasetRecordsWithLimits(raw, "json", "gtfobins", remoteDatasetParseLimits{MaxRecords: 2, StoreRecords: 2})
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if result.Total != 2 || len(result.Records) != 2 || !result.Truncated {
			t.Fatalf("result total/stored/truncated = %d/%d/%t", result.Total, len(result.Records), result.Truncated)
		}
		if result.Records[0].Comm != "alpha" || result.Records[1].Comm != "middle" {
			t.Fatalf("record order = %q, %q", result.Records[0].Comm, result.Records[1].Comm)
		}
	}
}

func TestPullRemoteDatasetReportsPreviewAndSafetyTruncation(t *testing.T) {
	raw := "{\"commandLine\":\"echo 1\"}\n{\"commandLine\":\"echo 2\"}\n{\"commandLine\":\"echo 3\"}\n{\"commandLine\":\"echo 4\"}\n{\"commandLine\":\"echo 5\"}\n"

	preview, err := pullRemoteDatasetWithRecordLimit(remoteDatasetRequest{
		Content:    raw,
		SourceName: "preview.jsonl",
		Format:     "jsonl",
		Limit:      2,
	}, nil, 10)
	if err != nil {
		t.Fatalf("preview error: %v", err)
	}
	if preview.Total != 5 || len(preview.Rows) != 2 || !preview.Truncated || preview.TotalIsLowerBound {
		t.Fatalf("preview total/rows/truncated/lower = %d/%d/%t/%t", preview.Total, len(preview.Rows), preview.Truncated, preview.TotalIsLowerBound)
	}

	imported, err := pullRemoteDatasetWithRecordLimit(remoteDatasetRequest{
		Content:    raw,
		SourceName: "import.jsonl",
		Format:     "jsonl",
		Limit:      2,
		ImportAll:  true,
	}, nil, 3)
	if err != nil {
		t.Fatalf("importAll error: %v", err)
	}
	if imported.Total != 3 || len(imported.Rows) != 3 || !imported.Truncated || !imported.TotalIsLowerBound || imported.RecordLimit != 3 {
		t.Fatalf("import total/rows/truncated/lower/limit = %d/%d/%t/%t/%d", imported.Total, len(imported.Rows), imported.Truncated, imported.TotalIsLowerBound, imported.RecordLimit)
	}
	foundRecordWarning := false
	for _, warning := range imported.ParseWarnings {
		if warning.Reason == "record_limit_truncated" {
			foundRecordWarning = true
		}
	}
	if !foundRecordWarning {
		t.Fatalf("parse warnings = %#v, want record_limit_truncated", imported.ParseWarnings)
	}
}

func TestParseRemoteDatasetRecordsWithLimitsCountsWithoutRetainingAll(t *testing.T) {
	result, err := parseRemoteDatasetRecordsWithLimits(
		[]byte("echo one\necho two\necho three\necho four\n"),
		"text",
		"inline.txt",
		remoteDatasetParseLimits{MaxRecords: 3, StoreRecords: 2},
	)
	if err != nil {
		t.Fatalf("parseRemoteDatasetRecordsWithLimits() error = %v", err)
	}
	if result.Format != "text" || result.Total != 3 || len(result.Records) != 2 || !result.Truncated {
		t.Fatalf("parse result = %#v, want three counted, two retained, and truncated", result)
	}
	if result.Records[0].Comm != "echo" || strings.Join(result.Records[1].Args, " ") != "two" {
		t.Fatalf("retained records = %#v", result.Records)
	}
}

func TestParseJSONDatasetLimitCountsValidRecordsAfterInvalidCandidates(t *testing.T) {
	raw := []byte(`{
		"rows": [
			{"metadata":"skip-1"},
			{"metadata":"skip-2"},
			{"metadata":"skip-3"},
			{"metadata":"skip-4"},
			{"commandLine":"echo one"},
			{"commandLine":"echo two"},
			{"commandLine":"echo three"}
		]
	}`)
	result, err := parseRemoteDatasetRecordsWithLimits(raw, "json", "inline.json", remoteDatasetParseLimits{
		MaxRecords:   2,
		StoreRecords: 2,
	})
	if err != nil {
		t.Fatalf("parseRemoteDatasetRecordsWithLimits() error = %v", err)
	}
	if result.Total != 2 || len(result.Records) != 2 || !result.Truncated {
		t.Fatalf("parse result = %#v, want two valid retained records and truncation", result)
	}
	if result.Records[0].CommandLine != "echo one" || result.Records[1].CommandLine != "echo two" {
		t.Fatalf("valid records after skipped candidates = %#v", result.Records)
	}
}

func TestFlattenDatasetJSONIsDeterministicBoundedAndNonMutating(t *testing.T) {
	decoded := map[string]any{
		"executables": map[string]any{
			"z-tool": map[string]any{"command": "z-tool run"},
			"a-tool": map[string]any{"command": "a-tool run"},
		},
	}
	executables := decoded["executables"].(map[string]any)
	items, truncated := flattenDatasetJSONWithLimit(decoded, 1)
	if !truncated || len(items) != 1 {
		t.Fatalf("flatten result = %#v truncated=%t, want one bounded candidate", items, truncated)
	}
	first, ok := items[0].(map[string]any)
	if !ok || first["_injected_name"] != "a-tool" {
		t.Fatalf("first deterministic item = %#v, want a-tool", items[0])
	}
	for name, value := range executables {
		if _, mutated := value.(map[string]any)["_injected_name"]; mutated {
			t.Fatalf("source object %q was mutated during flattening", name)
		}
	}
}

func TestPullRemoteDatasetRecordLimitCannotBeBypassedByImportAll(t *testing.T) {
	resp, err := pullRemoteDatasetWithRecordLimit(remoteDatasetRequest{
		Content:    "echo one\necho two\necho three\necho four\n",
		SourceName: "inline.txt",
		Format:     "text",
		Limit:      1,
		ImportAll:  true,
	}, nil, 3)
	if err != nil {
		t.Fatalf("pullRemoteDatasetWithRecordLimit() error = %v", err)
	}
	if resp.Total != 3 || len(resp.Rows) != 3 || resp.RecordLimit != 3 {
		t.Fatalf("response totals = total:%d rows:%d recordLimit:%d, want 3/3/3", resp.Total, len(resp.Rows), resp.RecordLimit)
	}
	if !resp.Truncated || !resp.TotalIsLowerBound {
		t.Fatalf("response truncation = truncated:%t lowerBound:%t, want true/true", resp.Truncated, resp.TotalIsLowerBound)
	}
	foundRecordLimitWarning := false
	for _, warning := range resp.ParseWarnings {
		if warning.Reason == "record_limit_truncated" {
			foundRecordLimitWarning = true
			break
		}
	}
	if !foundRecordLimitWarning {
		t.Fatalf("parse warnings = %#v, want record_limit_truncated", resp.ParseWarnings)
	}
}

func TestPullRemoteDatasetFromHTTPTransport(t *testing.T) {
	client := &http.Client{Transport: testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("method = %q, want GET", req.Method)
		}
		if got, want := req.URL.String(), "https://datasets.test/sample.json"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		if got := req.Header.Get("User-Agent"); got != "agent-ebpf-filter/1.0" {
			t.Fatalf("User-Agent = %q", got)
		}
		return newTestHTTPResponse(req, http.StatusOK, "application/json", `[
			{"commandLine":"sudo systemctl disable firewalld","label":"ALERT"},
			{"commandLine":"ls -la /tmp","label":"ALLOW"}
		]`), nil
	})}

	resp, err := pullRemoteDatasetWithClient(remoteDatasetRequest{
		URL:       "https://datasets.test/sample.json",
		Format:    "auto",
		Limit:     10,
		LabelMode: "preserve",
	}, client)
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Format != "json" {
		t.Fatalf("format = %q, want json", resp.Format)
	}
	if resp.Total != 2 || len(resp.Rows) != 2 {
		t.Fatalf("response rows = %d/%d, want 2/2", len(resp.Rows), resp.Total)
	}
	if resp.Rows[0].Label != "ALERT" || resp.Rows[0].Comm != "sudo" {
		t.Fatalf("first row = %#v", resp.Rows[0])
	}
}

func TestDownloadRemoteDatasetRejectsNonSuccessAndKnownOversizeResponses(t *testing.T) {
	tests := []struct {
		name      string
		response  *http.Response
		wantError string
	}{
		{
			name:      "non-success",
			response:  newTestHTTPResponse(nil, http.StatusBadGateway, "text/plain", "upstream failed"),
			wantError: "502 Bad Gateway",
		},
		{
			name: "known oversize",
			response: &http.Response{
				StatusCode:    http.StatusOK,
				Status:        "200 OK",
				Header:        make(http.Header),
				Body:          http.NoBody,
				ContentLength: remoteDatasetFetchLimitBytes + 1,
			},
			wantError: "larger than",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				tt.response.Request = req
				return tt.response, nil
			})}
			_, _, err := downloadRemoteDatasetWithClient("https://datasets.test/sample.json", client)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantError)
			}
		})
	}
}

func TestValidateRemoteDatasetURLSecurityBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "public HTTPS", rawURL: "https://example.com/data.json"},
		{name: "public nonstandard port", rawURL: "https://example.com:8443/data.json"},
		{name: "public IP literal", rawURL: "https://93.184.216.34/data.json"},
		{name: "file scheme", rawURL: "file:///etc/passwd", wantErr: true},
		{name: "missing host", rawURL: "https:///data.json", wantErr: true},
		{name: "userinfo", rawURL: "https://user:secret@example.com/data.json", wantErr: true},
		{name: "loopback literal", rawURL: "http://127.0.0.1/data.json", wantErr: true},
		{name: "mapped loopback literal", rawURL: "http://[::ffff:127.0.0.1]/data.json", wantErr: true},
		{name: "private literal", rawURL: "http://192.168.2.1/data.json", wantErr: true},
		{name: "link local literal", rawURL: "http://169.254.169.254/latest/meta-data", wantErr: true},
		{name: "IPv6 zone", rawURL: "http://[fe80::1%25eth0]/data.json", wantErr: true},
		{name: "invalid port", rawURL: "https://example.com:70000/data.json", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateRemoteDatasetURL(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateRemoteDatasetURL(%q) error = %v, wantErr=%t", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}

func TestBindRemoteDatasetRequestRejectsOversizeJSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/dataset", func(c *gin.Context) {
		if _, ok := bindRemoteDatasetRequestWithLimit(c, 64); ok {
			c.Status(http.StatusNoContent)
		}
	})

	body := `{"content":"` + strings.Repeat("x", 128) + `"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/dataset", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestRemoteDatasetAddressPolicyRejectsSpecialUseNetworks(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{address: "93.184.216.34", want: true},
		{address: "2606:4700:4700::1111", want: true},
		{address: "127.0.0.1"},
		{address: "10.0.0.1"},
		{address: "100.64.0.1"},
		{address: "169.254.169.254"},
		{address: "192.0.2.1"},
		{address: "198.18.0.1"},
		{address: "203.0.113.1"},
		{address: "168.63.129.16"},
		{address: "::1"},
		{address: "fc00::1"},
		{address: "fe80::1"},
		{address: "64:ff9b::7f00:1"},
		{address: "2001:db8::1"},
		{address: "::ffff:127.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			if got := isPublicRemoteDatasetAddress(netip.MustParseAddr(tt.address)); got != tt.want {
				t.Fatalf("isPublicRemoteDatasetAddress(%s) = %t, want %t", tt.address, got, tt.want)
			}
		})
	}
}

func TestRemoteDatasetDialContextPinsValidatedPublicAddresses(t *testing.T) {
	dialErr := errors.New("dial stopped by test")
	resolver := &staticRemoteDatasetResolver{addresses: map[string][]netip.Addr{
		"datasets.test": {
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("2606:4700:4700::1111"),
		},
	}}
	dialer := &recordingRemoteDatasetDialer{err: dialErr}
	dial := newRemoteDatasetDialContext(resolver, dialer)

	_, err := dial(context.Background(), "tcp", "datasets.test:443")
	if !errors.Is(err, dialErr) {
		t.Fatalf("dial error = %v, want wrapped test error", err)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "datasets.test" {
		t.Fatalf("resolver calls = %#v", resolver.calls)
	}
	if len(dialer.addresses) != 2 {
		t.Fatalf("dial addresses = %#v, want two public IP attempts", dialer.addresses)
	}
	for _, address := range dialer.addresses {
		host, _, splitErr := net.SplitHostPort(address)
		if splitErr != nil {
			t.Fatalf("dial address %q is invalid: %v", address, splitErr)
		}
		if host == "datasets.test" {
			t.Fatalf("dialer received unpinned hostname %q", address)
		}
		if parsed, parseErr := netip.ParseAddr(host); parseErr != nil || !isPublicRemoteDatasetAddress(parsed) {
			t.Fatalf("dialer received invalid address %q", address)
		}
	}
}

func TestRemoteDatasetDialContextFailsClosedOnMixedDNSAnswers(t *testing.T) {
	resolver := &staticRemoteDatasetResolver{addresses: map[string][]netip.Addr{
		"datasets.test": {
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("192.168.2.1"),
		},
	}}
	dialer := &recordingRemoteDatasetDialer{err: errors.New("must not dial")}

	_, err := newRemoteDatasetDialContext(resolver, dialer)(context.Background(), "tcp", "datasets.test:443")
	if err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("dial error = %v, want non-public DNS rejection", err)
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("dialer was called with %#v after mixed DNS response", dialer.addresses)
	}
}

func TestRemoteDatasetDialContextRevalidatesDNSForEveryConnection(t *testing.T) {
	dialErr := errors.New("dial stopped by test")
	resolver := &sequencedRemoteDatasetResolver{responses: [][]netip.Addr{
		{netip.MustParseAddr("93.184.216.34")},
		{netip.MustParseAddr("127.0.0.1")},
	}}
	dialer := &recordingRemoteDatasetDialer{err: dialErr}
	dial := newRemoteDatasetDialContext(resolver, dialer)

	if _, err := dial(context.Background(), "tcp", "datasets.test:443"); !errors.Is(err, dialErr) {
		t.Fatalf("first dial error = %v, want wrapped test error", err)
	}
	if _, err := dial(context.Background(), "tcp", "datasets.test:443"); err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("second dial error = %v, want DNS rebinding rejection", err)
	}
	if resolver.calls != 2 {
		t.Fatalf("resolver calls = %d, want 2", resolver.calls)
	}
	if len(dialer.addresses) != 1 {
		t.Fatalf("dial addresses = %#v, private rebound address must not be dialed", dialer.addresses)
	}
}

func TestRemoteDatasetDialContextRejectsEmptyDNSResponse(t *testing.T) {
	resolver := &staticRemoteDatasetResolver{addresses: map[string][]netip.Addr{}}
	dialer := &recordingRemoteDatasetDialer{err: errors.New("must not dial")}

	_, err := newRemoteDatasetDialContext(resolver, dialer)(context.Background(), "tcp", "datasets.test:443")
	if err == nil || !strings.Contains(err.Error(), "did not resolve") {
		t.Fatalf("dial error = %v, want empty DNS rejection", err)
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("dialer was called with %#v after empty DNS response", dialer.addresses)
	}
}

func TestRemoteDatasetHTTPClientDisablesProxyAndRejectsUnsafeRedirects(t *testing.T) {
	client := newRemoteDatasetHTTPClient(&staticRemoteDatasetResolver{}, &recordingRemoteDatasetDialer{})
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("remote dataset transport must not use environment proxies")
	}
	if transport.DialContext == nil || client.CheckRedirect == nil {
		t.Fatal("remote dataset client is missing hardened dial or redirect policy")
	}

	httpsRequest, _ := http.NewRequest(http.MethodGet, "https://example.com/data.json", nil)
	httpRequest, _ := http.NewRequest(http.MethodGet, "http://example.com/data.json", nil)
	if err := client.CheckRedirect(httpRequest, []*http.Request{httpsRequest}); err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("HTTPS downgrade error = %v", err)
	}
	privateRequest, _ := http.NewRequest(http.MethodGet, "https://127.0.0.1/data.json", nil)
	if err := client.CheckRedirect(privateRequest, []*http.Request{httpsRequest}); err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("private redirect error = %v", err)
	}
	redirects := make([]*http.Request, remoteDatasetRedirectLimit)
	for i := range redirects {
		redirects[i] = httpsRequest
	}
	if err := client.CheckRedirect(httpsRequest, redirects); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Fatalf("redirect limit error = %v", err)
	}
}

func TestReadLimitedRemoteDatasetPayloadRejectsExpandedOversize(t *testing.T) {
	payload := bytes.NewReader(make([]byte, remoteDatasetFetchLimitBytes+1))
	_, err := readLimitedRemoteDatasetPayload(payload, "compressed payload")
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("error = %v, want expanded payload size rejection", err)
	}
}

func TestExpandRemoteDatasetPayloadsEnforcesCumulativeArchiveBudget(t *testing.T) {
	archive := buildZipArchive(t, map[string]string{
		"a.txt": "123456",
		"b.txt": "abcdef",
	})

	_, err := expandRemoteDatasetPayloadsWithBudget(archive, "application/zip", "samples.zip", 0, newRemoteDatasetArchiveBudget(10, 10, 4))
	if !errors.Is(err, errRemoteDatasetArchiveBudgetExceeded) {
		t.Fatalf("error = %v, want cumulative archive budget rejection", err)
	}

	payloads, err := expandRemoteDatasetPayloadsWithBudget(archive, "application/zip", "samples.zip", 0, newRemoteDatasetArchiveBudget(12, 2, 4))
	if err != nil {
		t.Fatalf("exact archive budget returned error: %v", err)
	}
	if len(payloads) != 2 {
		t.Fatalf("payload count = %d, want 2", len(payloads))
	}
}

func TestExpandRemoteDatasetPayloadsChargesCorruptMembersAgainstBudget(t *testing.T) {
	contents := []string{"corrupt-first-member", "corrupt-second-member"}
	archive := buildZipArchiveEntries(t, []zipTestEntry{
		{Name: "first.txt", Content: contents[0], Method: zip.Store},
		{Name: "second.txt", Content: contents[1], Method: zip.Store},
	})
	for _, content := range contents {
		index := bytes.Index(archive, []byte(content))
		if index < 0 {
			t.Fatalf("stored zip member %q was not found", content)
		}
		archive[index] ^= 0xff
	}

	maxBytes := int64(len(contents[0]) + len(contents[1]) - 1)
	_, warnings, err := expandRemoteDatasetPayloadsWithBudgetAndWarnings(
		archive,
		"application/zip",
		"corrupt.zip",
		0,
		newRemoteDatasetArchiveBudget(maxBytes, 10, 4),
	)
	if !errors.Is(err, errRemoteDatasetArchiveBudgetExceeded) {
		t.Fatalf("error = %v, want corrupt members to consume the cumulative archive budget", err)
	}
	if len(warnings) != 1 || warnings[0].Source != "corrupt.zip!first.txt" ||
		!strings.HasPrefix(warnings[0].Reason, "archive_member_read_failed:") {
		t.Fatalf("warnings = %#v, want first corrupt member warning before budget rejection", warnings)
	}
}

func TestExpandRemoteDatasetPayloadsCountsSkippedArchiveMembers(t *testing.T) {
	archive := buildZipArchive(t, map[string]string{
		"README.md": "documentation",
		"a.txt":     "echo a",
		"b.txt":     "echo b",
	})

	_, err := expandRemoteDatasetPayloadsWithBudget(archive, "application/zip", "samples.zip", 0, newRemoteDatasetArchiveBudget(1024, 2, 4))
	if !errors.Is(err, errRemoteDatasetArchiveBudgetExceeded) {
		t.Fatalf("error = %v, want archive member budget rejection", err)
	}
}

func TestExpandRemoteDatasetPayloadsRejectsExcessiveNesting(t *testing.T) {
	data := []byte("echo hello\n")
	for range 5 {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("gzip write: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}
		data = compressed.Bytes()
	}

	_, err := expandRemoteDatasetPayloadsWithBudget(data, "application/gzip", "nested.gz", 0, newRemoteDatasetArchiveBudget(1<<20, 10, 4))
	if !errors.Is(err, errRemoteDatasetArchiveBudgetExceeded) {
		t.Fatalf("error = %v, want archive nesting rejection", err)
	}
}

func TestExpandRemoteDatasetPayloadsPropagatesNestedBudgetErrors(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte("12345678")); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	archive := buildZipArchive(t, map[string]string{
		"nested.gz": compressed.String(),
	})
	maxBytes := int64(compressed.Len() + 4)

	_, err := expandRemoteDatasetPayloadsWithBudget(archive, "application/zip", "nested.zip", 0, newRemoteDatasetArchiveBudget(maxBytes, 10, 4))
	if !errors.Is(err, errRemoteDatasetArchiveBudgetExceeded) {
		t.Fatalf("error = %v, want nested archive budget rejection", err)
	}
}

func TestExpandRemoteDatasetPayloadsChargesOversizeNestedArchivesAgainstBudget(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := io.CopyN(writer, zeroReader{}, int64(remoteDatasetFetchLimitBytes+1)); err != nil {
		t.Fatalf("gzip write oversized payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close oversized payload: %v", err)
	}
	archive := buildZipArchive(t, map[string]string{
		"first.gz":  compressed.String(),
		"second.gz": compressed.String(),
	})
	maxBytes := int64(remoteDatasetFetchLimitBytes+1) + 2*int64(compressed.Len())

	_, warnings, err := expandRemoteDatasetPayloadsWithBudgetAndWarnings(
		archive,
		"application/zip",
		"nested.zip",
		0,
		newRemoteDatasetArchiveBudget(maxBytes, 10, 4),
	)
	if !errors.Is(err, errRemoteDatasetArchiveBudgetExceeded) {
		t.Fatalf("error = %v, want repeated oversized nested archive budget rejection", err)
	}
	if len(warnings) != 1 || !strings.HasPrefix(warnings[0].Reason, "nested_archive_decode_failed:") {
		t.Fatalf("warnings = %#v, want one nested oversize warning before budget rejection", warnings)
	}
}

func TestExpandTarRemoteDatasetPayloadRejectsMalformedArchiveWithoutLooping(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		_, err := expandTarRemoteDatasetPayloadWithBudget([]byte("truncated tar archive"), "broken.tar", 0, newRemoteDatasetArchiveBudget(1024, 10, 4))
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("malformed tar returned no error")
		}
	case <-time.After(time.Second):
		t.Fatal("malformed tar processing did not terminate")
	}
}

func TestArchiveContentTypeDetectionDoesNotConfuseGzipWithZip(t *testing.T) {
	data := []byte{0x1f, 0x8b, 0x08, 0x00}
	if isZipPayload(data, "application/gzip", "sample.gz") {
		t.Fatal("gzip payload was misclassified as zip")
	}
	if !isGzipPayload(data, "application/gzip; charset=binary", "sample.bin") {
		t.Fatal("gzip payload was not detected from media type")
	}
}

func TestPullRemoteDatasetFromClassicCSVWithCleaning(t *testing.T) {
	raw := []byte(`payload,length,attack_type,label
"c/ caridad s/n",14,norm,norm
"../etc/passwd",12,cmdi,anom
`)

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		Content:        string(raw),
		SourceName:     "HttpParamsDataset/payload_train.csv",
		Format:         "csv",
		Limit:          10,
		LabelMode:      "preserve",
		CleanSensitive: true,
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Format != "csv" {
		t.Fatalf("format = %q, want csv", resp.Format)
	}
	if len(resp.Rows) != 2 {
		t.Fatalf("rows length = %d, want 2", len(resp.Rows))
	}
	if resp.Rows[0].Label != "ALLOW" {
		t.Fatalf("first row label = %q, want ALLOW", resp.Rows[0].Label)
	}
	if resp.Rows[1].Label != "BLOCK" {
		t.Fatalf("second row label = %q, want BLOCK", resp.Rows[1].Label)
	}
	if strings.Contains(resp.Rows[1].CommandLine, "/etc/passwd") {
		t.Fatalf("sensitive path was not cleaned: %#v", resp.Rows[1])
	}
	if resp.Rows[1].Source != "HttpParamsDataset/payload_train.csv" {
		t.Fatalf("row source = %q, want dataset source", resp.Rows[1].Source)
	}
}

func TestPullRemoteDatasetRejectsHTMLLandingPage(t *testing.T) {
	client := &http.Client{Transport: testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestHTTPResponse(req, http.StatusOK, "text/plain; charset=utf-8", `<!DOCTYPE html>
<html lang=en dir=ltr prefix=content: http://purl.org/rss/1.0/modules/content/ dc: http://purl.org/dc/terms/>
<head><title>Dataset</title></head>
<body>Download page</body>
</html>`), nil
	})}

	_, err := pullRemoteDatasetWithClient(remoteDatasetRequest{
		URL:    "https://datasets.test/landing",
		Format: "auto",
		Limit:  10,
	}, client)
	if err == nil {
		t.Fatalf("pullRemoteDataset() error = nil, want HTML landing page rejection")
	}
	if got := err.Error(); !strings.Contains(got, "HTML landing page") {
		t.Fatalf("error = %q, want HTML landing page rejection", got)
	}
}

func TestPullRemoteDatasetFromBase64ZipArchive(t *testing.T) {
	archiveBytes := buildZipArchive(t, map[string]string{
		"README.md":     "# Dataset\nThis is documentation and should be skipped.\n",
		"samples.jsonl": `{"commandLine":"rm -rf /tmp/demo","label":"BLOCK"}` + "\n" + `{"commandLine":"echo hello","label":"ALLOW"}` + "\n",
	})

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		ContentBase64: base64.StdEncoding.EncodeToString(archiveBytes),
		SourceName:    "classic.zip",
		Format:        "auto",
		Limit:         10,
		LabelMode:     "preserve",
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Source != "classic.zip" {
		t.Fatalf("Source = %q, want classic.zip", resp.Source)
	}
	if resp.Total != 2 || len(resp.Rows) != 2 {
		t.Fatalf("response rows = %d/%d, want 2/2", len(resp.Rows), resp.Total)
	}
	if resp.Rows[0].Comm != "rm" || resp.Rows[1].Comm != "echo" {
		t.Fatalf("rows = %#v %#v", resp.Rows[0], resp.Rows[1])
	}
}

func TestPullRemoteDatasetReportsArchiveMemberFailures(t *testing.T) {
	const corruptContent = "corrupt-member-payload"

	tests := []struct {
		name          string
		buildArchive  func(*testing.T) []byte
		warningSource string
		warningReason string
	}{
		{
			name: "open",
			buildArchive: func(t *testing.T) []byte {
				return buildZipArchiveEntries(t, []zipTestEntry{
					{Name: "good.txt", Content: "echo ok\n", Method: zip.Store},
					{Name: "unsupported.txt", Content: "ignored", Method: 99},
				})
			},
			warningSource: "mixed.zip!unsupported.txt",
			warningReason: "archive_member_open_failed:",
		},
		{
			name: "read",
			buildArchive: func(t *testing.T) []byte {
				archive := buildZipArchiveEntries(t, []zipTestEntry{
					{Name: "good.txt", Content: "echo ok\n", Method: zip.Store},
					{Name: "corrupt.txt", Content: corruptContent, Method: zip.Store},
				})
				index := bytes.Index(archive, []byte(corruptContent))
				if index < 0 {
					t.Fatal("stored zip member content was not found")
				}
				archive[index] ^= 0xff
				return archive
			},
			warningSource: "mixed.zip!corrupt.txt",
			warningReason: "archive_member_read_failed:",
		},
		{
			name: "nested_decode",
			buildArchive: func(t *testing.T) []byte {
				return buildZipArchiveEntries(t, []zipTestEntry{
					{Name: "good.txt", Content: "echo ok\n", Method: zip.Store},
					{Name: "broken.gz", Content: "not a gzip stream", Method: zip.Store},
				})
			},
			warningSource: "mixed.zip!broken.gz",
			warningReason: "nested_archive_decode_failed:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := pullRemoteDataset(remoteDatasetRequest{
				ContentBase64: base64.StdEncoding.EncodeToString(tt.buildArchive(t)),
				SourceName:    "mixed.zip",
				Format:        "auto",
				Limit:         10,
			})
			if err != nil {
				t.Fatalf("pullRemoteDataset() error = %v", err)
			}
			if resp.Total != 1 || len(resp.Rows) != 1 || resp.Rows[0].Comm != "echo" {
				t.Fatalf("response rows = total:%d rows:%#v, want one good record", resp.Total, resp.Rows)
			}
			if len(resp.ParseWarnings) != 1 {
				t.Fatalf("parse warnings = %#v, want one archive warning", resp.ParseWarnings)
			}
			warning := resp.ParseWarnings[0]
			if warning.Source != tt.warningSource || warning.Count != 1 || !strings.HasPrefix(warning.Reason, tt.warningReason) {
				t.Fatalf("parse warning = %#v, want source %q and reason prefix %q", warning, tt.warningSource, tt.warningReason)
			}
		})
	}
}

func TestPullRemoteDatasetReportsTarMemberReadFailure(t *testing.T) {
	resp, err := pullRemoteDataset(remoteDatasetRequest{
		ContentBase64: base64.StdEncoding.EncodeToString(buildTruncatedTarArchive(t)),
		SourceName:    "mixed.tar",
		Format:        "auto",
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Total != 1 || len(resp.Rows) != 1 || resp.Rows[0].Comm != "echo" {
		t.Fatalf("response rows = total:%d rows:%#v, want one good record", resp.Total, resp.Rows)
	}
	if len(resp.ParseWarnings) != 1 {
		t.Fatalf("parse warnings = %#v, want one archive warning", resp.ParseWarnings)
	}
	warning := resp.ParseWarnings[0]
	if warning.Source != "mixed.tar!corrupt.txt" || warning.Count != 1 || !strings.HasPrefix(warning.Reason, "archive_member_read_failed:") {
		t.Fatalf("parse warning = %#v, want tar member read warning", warning)
	}
}

func TestPullRemoteDatasetReportsTarStreamReadFailureAfterValidMember(t *testing.T) {
	resp, err := pullRemoteDataset(remoteDatasetRequest{
		ContentBase64: base64.StdEncoding.EncodeToString(buildTarArchiveWithTruncatedNextHeader(t)),
		SourceName:    "mixed.tar",
		Format:        "auto",
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Total != 1 || len(resp.Rows) != 1 || resp.Rows[0].Comm != "echo" {
		t.Fatalf("response rows = total:%d rows:%#v, want one good record", resp.Total, resp.Rows)
	}
	if len(resp.ParseWarnings) != 1 {
		t.Fatalf("parse warnings = %#v, want one archive stream warning", resp.ParseWarnings)
	}
	warning := resp.ParseWarnings[0]
	if warning.Source != "mixed.tar" || warning.Count != 1 || !strings.HasPrefix(warning.Reason, "archive_stream_read_failed:") {
		t.Fatalf("parse warning = %#v, want tar stream read warning", warning)
	}
}

func TestPullRemoteDatasetFromTarGzArchive(t *testing.T) {
	tarBytes := buildTarArchive(t, map[string]string{
		"commands.txt": "sudo systemctl disable firewalld\nls -la /tmp\n",
	})
	var gz bytes.Buffer
	gw := gzip.NewWriter(&gz)
	if _, err := gw.Write(tarBytes); err != nil {
		t.Fatalf("gzip write error = %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close error = %v", err)
	}

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		ContentBase64: base64.StdEncoding.EncodeToString(gz.Bytes()),
		SourceName:    "classic.tar.gz",
		Format:        "auto",
		Limit:         10,
		LabelMode:     "preserve",
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Total != 2 || len(resp.Rows) != 2 {
		t.Fatalf("response rows = %d/%d, want 2/2", len(resp.Rows), resp.Total)
	}
	if resp.Rows[0].Comm != "sudo" || resp.Rows[1].Comm != "ls" {
		t.Fatalf("rows = %#v %#v", resp.Rows[0], resp.Rows[1])
	}
}
