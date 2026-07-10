package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ulikunitz/xz"
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

func TestPullRemoteDatasetFromTarXzArchive(t *testing.T) {
	tarBytes := buildTarArchive(t, map[string]string{
		"commands.txt": "sudo systemctl disable firewalld\nls -la /tmp\n",
	})
	var xzBuf bytes.Buffer
	xw, err := xz.NewWriter(&xzBuf)
	if err != nil {
		t.Fatalf("xz writer error = %v", err)
	}
	if _, err := xw.Write(tarBytes); err != nil {
		t.Fatalf("xz write error = %v", err)
	}
	if err := xw.Close(); err != nil {
		t.Fatalf("xz close error = %v", err)
	}

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		ContentBase64: base64.StdEncoding.EncodeToString(xzBuf.Bytes()),
		SourceName:    "classic.tar.xz",
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

func TestParseRemoteDatasetRecordsGTFOBinsAndLOLBAS(t *testing.T) {
	// GTFOBins style: real API shape uses top-level executables map.
	gtfoRaw := []byte(`{
		"functions": {
			"shell": { "label": "Shell" }
		},
		"contexts": {
			"sudo": { "label": "Sudo" }
		},
		"executables": {
			"7z": {
				"functions": {
					"file-read": [
						{ "code": "7z a -ttar -an -so /etc/shadow | 7z e -ttar -si -so" }
					]
				}
			},
			"comm": {
				"functions": {
					"shell": [
						{ "code": "comm /tmp/a /tmp/b" }
					]
				}
			}
		}
	}`)
	records, _, err := parseRemoteDatasetRecords(gtfoRaw, "auto", "GTFOBins")
	if err != nil {
		t.Fatalf("GTFOBins parse error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("GTFOBins record count = %d, want 2", len(records))
	}
	got := map[string]remoteDatasetRecord{}
	for _, rec := range records {
		if strings.HasPrefix(rec.CommandLine, "{") {
			t.Fatalf("GTFOBins record command line is still serialized JSON: %#v", rec)
		}
		got[rec.Comm] = rec
	}
	if got["7z"].Category != "file-read" || got["7z"].CommandLine != "7z a -ttar -an -so /etc/shadow | 7z e -ttar -si -so" {
		t.Fatalf("GTFOBins 7z record = %#v", got["7z"])
	}
	if got["comm"].Category != "shell" || got["comm"].CommandLine != "comm /tmp/a /tmp/b" {
		t.Fatalf("GTFOBins comm record = %#v", got["comm"])
	}

	// LOLBAS style
	lolbasRaw := []byte(`[
		{
			"Name": "7z.exe",
			"Commands": [
				{ "Command": "7z.exe a -ttar -an -so /etc/shadow", "Category": "Download" }
			]
		}
	]`)
	records, _, err = parseRemoteDatasetRecords(lolbasRaw, "auto", "LOLBAS")
	if err != nil {
		t.Fatalf("LOLBAS parse error = %v", err)
	}
	if len(records) != 1 || records[0].Comm != "7z.exe" || records[0].Category != "Download" {
		t.Fatalf("LOLBAS record = %#v", records[0])
	}
}

func TestParseRemoteDatasetRecordsSpecialSerialization(t *testing.T) {
	// Object that isn't expanded but is picked up as a value
	raw := []byte(`[
		{
			"comm": "test-binary",
			"metadata": { "author": "me", "version": 1.0 }
		}
	]`)
	records, _, err := parseRemoteDatasetRecords(raw, "auto", "inline.json")
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	_ = records
	// If we looked for 'metadata' as a string, it should now be a JSON string
	val := firstStringValue(map[string]any{"m": map[string]any{"a": 1}}, "m")
	if val != `{"a":1}` {
		t.Fatalf("got %q, want {\"a\":1}", val)
	}
}

func TestParseRemoteDatasetRecordsTextNumericSequencePreserved(t *testing.T) {
	raw := []byte("1 2 3 4\n5 6 7\n")
	records, format, err := parseRemoteDatasetRecords(raw, "auto", "ADFA-LD.txt")
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if format != "text" {
		t.Fatalf("format = %q, want text", format)
	}
	if len(records) != 2 {
		t.Fatalf("records length = %d, want 2", len(records))
	}
	if records[0].Comm != "syscall-seq" || strings.Join(records[0].Args, " ") != "1 2 3 4" {
		t.Fatalf("first record = %#v", records[0])
	}
	if records[1].Comm != "syscall-seq" || strings.Join(records[1].Args, " ") != "5 6 7" {
		t.Fatalf("second record = %#v", records[1])
	}
}

func TestParseRemoteDatasetRecordsSafetyNetRules(t *testing.T) {
	raw := []byte(`{
		"source": "github.com/kenryu42/claude-code-safety-net",
		"rules": [
			{
				"command": "git reset --hard HEAD~1",
				"action": "BLOCK",
				"priority": 200,
				"reason": "test"
			}
		]
	}`)
	records, format, err := parseRemoteDatasetRecords(raw, "auto", "Claude Code Safety Net")
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if format != "json" {
		t.Fatalf("format = %q, want json", format)
	}
	if len(records) != 1 {
		t.Fatalf("records length = %d, want 1", len(records))
	}
	if records[0].Comm != "git" || strings.Join(records[0].Args, " ") != "reset --hard HEAD~1" {
		t.Fatalf("record = %#v", records[0])
	}
	if records[0].Label != "BLOCK" {
		t.Fatalf("label = %q, want BLOCK", records[0].Label)
	}
}

func TestParseRemoteDatasetRecordsTextSkipsCommentNoise(t *testing.T) {
	raw := []byte("/*\n* This file contains the system call numbers, based on the\n__SYSCALL(__NR_io_setup, sys_io_setup)\necho hello\n")
	records, format, err := parseRemoteDatasetRecords(raw, "auto", "noisy.txt")
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if format != "text" {
		t.Fatalf("format = %q, want text", format)
	}
	if len(records) != 1 {
		t.Fatalf("records length = %d, want 1", len(records))
	}
	if records[0].Comm != "echo" || strings.Join(records[0].Args, " ") != "hello" {
		t.Fatalf("record = %#v", records[0])
	}
}

func TestParseRemoteDatasetRecordsSELinuxPolicyText(t *testing.T) {
	raw := []byte(`
# Common SELinux .te policy text should import as labeled samples.
allow httpd_t httpd_sys_content_t:file {
	getattr
	open
	read
};
neverallow domain shadow_t:file write;
dontaudit httpd_t user_home_t:dir search;
type_transition httpd_t tmp_t:file httpd_tmp_t;
`)

	records, format, err := parseRemoteDatasetRecords(raw, "auto", "local-policy.te")
	if err != nil {
		t.Fatalf("parseRemoteDatasetRecords() error = %v", err)
	}
	if format != "text" {
		t.Fatalf("format = %q, want text", format)
	}
	if len(records) != 4 {
		t.Fatalf("records length = %d, want 4: %#v", len(records), records)
	}

	wantLabels := []string{"ALLOW", "BLOCK", "ALERT", "ALLOW"}
	for i, record := range records {
		if record.Comm != "selinux-rule" {
			t.Fatalf("record[%d].Comm = %q, want selinux-rule: %#v", i, record.Comm, record)
		}
		if record.Label != wantLabels[i] {
			t.Fatalf("record[%d].Label = %q, want %q: %#v", i, record.Label, wantLabels[i], record)
		}
		if record.Category != "SELINUX_POLICY" || record.UserLabel != "selinux-policy" {
			t.Fatalf("record[%d] category/userLabel mismatch: %#v", i, record)
		}
	}
	if !strings.Contains(records[0].CommandLine, "getattr open read") {
		t.Fatalf("multiline allow rule was not collapsed: %q", records[0].CommandLine)
	}
}

func TestPullRemoteDatasetSELinuxJSONRulesPreservesLabels(t *testing.T) {
	raw := `{
		"rules": [
			{"rule":"allow httpd_t http_port_t:tcp_socket name_connect;"},
			{"selinuxRule":"neverallow domain shadow_t:file write;", "action":"BLOCK"}
		]
	}`

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		Content:    raw,
		SourceName: "policy-rules.json",
		Format:     "auto",
		Limit:      10,
		LabelMode:  "preserve",
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Format != "json" {
		t.Fatalf("format = %q, want json", resp.Format)
	}
	if resp.Total != 2 || len(resp.Rows) != 2 {
		t.Fatalf("response rows = %d/%d, want 2/2", len(resp.Rows), resp.Total)
	}
	if resp.Rows[0].Comm != "selinux-rule" || resp.Rows[0].Label != "ALLOW" {
		t.Fatalf("first row = %#v", resp.Rows[0])
	}
	if resp.Rows[0].LabelSource != "selinux-policy-rule" {
		t.Fatalf("first row labelSource = %q, want selinux-policy-rule", resp.Rows[0].LabelSource)
	}
	if resp.Rows[1].Label != "BLOCK" || resp.Rows[1].UserLabel != "selinux-policy" {
		t.Fatalf("second row = %#v", resp.Rows[1])
	}

	sample := buildRemoteDatasetSample(resp.Rows[0], "preserve", false)
	if sample.UserLabel != "selinux-policy" || sample.Category != "SELINUX_POLICY" {
		t.Fatalf("sample metadata was not preserved: userLabel=%q category=%q", sample.UserLabel, sample.Category)
	}
	if len(resp.ByLabel) == 0 || len(resp.ByCategory) == 0 || len(resp.BySource) == 0 {
		t.Fatalf("dataset stats were not populated: labels=%#v categories=%#v sources=%#v", resp.ByLabel, resp.ByCategory, resp.BySource)
	}
	if resp.Quality.ImportableCount != 2 || resp.Quality.LabeledCount != 2 || resp.Normalization.FeatureDim != FeatureDim {
		t.Fatalf("dataset quality/normalization mismatch: quality=%#v normalization=%#v", resp.Quality, resp.Normalization)
	}
}

func TestPullRemoteDatasetStatsAndWarnings(t *testing.T) {
	archiveBytes := buildZipArchive(t, map[string]string{
		"README.md": "# no records here\n",
		"commands.txt": strings.Join([]string{
			"git status",
			"rm -rf /tmp/demo",
			"curl https://example.com/data.json",
		}, "\n"),
	})

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		ContentBase64: base64.StdEncoding.EncodeToString(archiveBytes),
		SourceName:    "mixed.zip",
		Format:        "auto",
		Limit:         2,
		LabelMode:     "heuristic",
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if !resp.Truncated || resp.Total != 3 || len(resp.Rows) != 2 {
		t.Fatalf("truncation mismatch: total=%d rows=%d truncated=%t", resp.Total, len(resp.Rows), resp.Truncated)
	}
	if len(resp.ParseWarnings) == 0 {
		t.Fatalf("expected parse warning for README/truncation, got none")
	}
	if resp.Normalization.FeatureDim != FeatureDim || resp.Quality.ImportableCount != 2 {
		t.Fatalf("stats mismatch quality=%#v normalization=%#v", resp.Quality, resp.Normalization)
	}
	if len(resp.ByLabel) == 0 || len(resp.ByCategory) == 0 || len(resp.BySource) == 0 {
		t.Fatalf("missing rollups labels=%#v categories=%#v sources=%#v", resp.ByLabel, resp.ByCategory, resp.BySource)
	}
}

func TestBuildRemoteDatasetSampleForceBlock(t *testing.T) {
	row := remoteDatasetRow{
		CommandLine: "openvt -- /bin/sh",
		Comm:        "openvt",
		Args:        []string{"--", "/bin/sh"},
		Label:       "ALLOW",
		Category:    "shell",
		Timestamp:   "2026-01-01T00:00:00Z",
		UserLabel:   "dataset",
	}
	sample := buildRemoteDatasetSample(row, "block", false)
	if sample.Label != 1 {
		t.Fatalf("sample.Label = %d, want BLOCK", sample.Label)
	}
	if sample.UserLabel != "remote-block" {
		t.Fatalf("sample.UserLabel = %q, want remote-block", sample.UserLabel)
	}
	if sample.CommandLine != row.CommandLine {
		t.Fatalf("sample.CommandLine = %q, want %q", sample.CommandLine, row.CommandLine)
	}
}

func TestBuildRemoteDatasetRowInfersLabelFromSource(t *testing.T) {
	record := remoteDatasetRecord{
		Row:         7,
		Source:      "mpsd/powershell_benign_dataset/sample.ps1",
		CommandLine: "Write-Host hello",
		Comm:        "Write-Host",
		Args:        []string{"hello"},
	}

	row := buildRemoteDatasetRow(record, "preserve", false)
	if row.Label != "ALLOW" {
		t.Fatalf("row.Label = %q, want ALLOW", row.Label)
	}
	if row.LabelSource != "source" {
		t.Fatalf("row.LabelSource = %q, want source", row.LabelSource)
	}
	if row.Source != record.Source {
		t.Fatalf("row.Source = %q, want %q", row.Source, record.Source)
	}
}

func TestBuildRemoteDatasetSampleCleansSensitiveValues(t *testing.T) {
	row := remoteDatasetRow{
		Source:      "HttpParamsDataset/payload_train.csv",
		CommandLine: "curl https://user:secret@example.com/path?token=abc123 -H \"Authorization: Bearer abc123\"",
		Comm:        "curl",
		Args:        []string{"https://user:secret@example.com/path?token=abc123", "-H", "Authorization: Bearer abc123"},
		Label:       "BLOCK",
		Category:    "NETWORK",
		Timestamp:   "2026-01-01T00:00:00Z",
		UserLabel:   "remote-source-label",
	}

	sample := buildRemoteDatasetSample(row, "preserve", true)
	if strings.Contains(sample.CommandLine, "secret") || strings.Contains(sample.CommandLine, "token=abc123") {
		t.Fatalf("sample.CommandLine still contains sensitive data: %q", sample.CommandLine)
	}
	if strings.Contains(strings.Join(sample.Args, " "), "secret") || strings.Contains(strings.Join(sample.Args, " "), "abc123") {
		t.Fatalf("sample.Args still contains sensitive data: %#v", sample.Args)
	}
	if !strings.Contains(sample.CommandLine, "***") {
		t.Fatalf("sample.CommandLine = %q, want masked content", sample.CommandLine)
	}
}

func TestNormalizeActionLabelClassicDatasetSynonyms(t *testing.T) {
	cases := map[string]string{
		"norm":                 "ALLOW",
		"BENIGN":               "ALLOW",
		"anom":                 "BLOCK",
		"cmdi":                 "BLOCK",
		"sql injection":        "BLOCK",
		"path traversal":       "BLOCK",
		"cross-site scripting": "BLOCK",
	}
	for input, want := range cases {
		if got := normalizeActionLabel(input); got != want {
			t.Fatalf("normalizeActionLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func buildZipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("zip create %q error = %v", name, err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %q error = %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip close error = %v", err)
	}
	return buf.Bytes()
}

func buildTarArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar write header %q error = %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar write %q error = %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close error = %v", err)
	}
	return buf.Bytes()
}
