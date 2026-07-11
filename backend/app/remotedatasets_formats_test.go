package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

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

func TestParseRemoteDatasetSELinuxRulePreservesNormalizedUserLabelKey(t *testing.T) {
	records, _, err := parseRemoteDatasetRecords(
		[]byte(`{"rule":"allow httpd_t http_port_t:tcp_socket name_connect;","userlabel":"curated-policy"}`),
		"json",
		"policy.json",
	)
	if err != nil {
		t.Fatalf("parseRemoteDatasetRecords() error = %v", err)
	}
	if len(records) != 1 || records[0].UserLabel != "curated-policy" {
		t.Fatalf("SELinux user label records = %#v", records)
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

type zipTestEntry struct {
	Name    string
	Content string
	Method  uint16
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func buildZipArchiveEntries(t *testing.T, entries []zipTestEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	w.RegisterCompressor(99, func(writer io.Writer) (io.WriteCloser, error) {
		return nopWriteCloser{Writer: writer}, nil
	})
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.Name, Method: entry.Method}
		fw, err := w.CreateHeader(header)
		if err != nil {
			t.Fatalf("zip create %q error = %v", entry.Name, err)
		}
		if _, err := fw.Write([]byte(entry.Content)); err != nil {
			t.Fatalf("zip write %q error = %v", entry.Name, err)
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

func buildTruncatedTarArchive(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	good := []byte("echo ok\n")
	if err := tw.WriteHeader(&tar.Header{Name: "good.txt", Mode: 0o600, Size: int64(len(good))}); err != nil {
		t.Fatalf("tar write good header error = %v", err)
	}
	if _, err := tw.Write(good); err != nil {
		t.Fatalf("tar write good payload error = %v", err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "corrupt.txt", Mode: 0o600, Size: 64}); err != nil {
		t.Fatalf("tar write corrupt header error = %v", err)
	}
	if _, err := tw.Write([]byte("truncated")); err != nil {
		t.Fatalf("tar write corrupt payload error = %v", err)
	}
	return buf.Bytes()
}

func buildTarArchiveWithTruncatedNextHeader(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	good := []byte("echo ok\n")
	if err := tw.WriteHeader(&tar.Header{Name: "good.txt", Mode: 0o600, Size: int64(len(good))}); err != nil {
		t.Fatalf("tar write good header error = %v", err)
	}
	if _, err := tw.Write(good); err != nil {
		t.Fatalf("tar write good payload error = %v", err)
	}
	buf.Write(bytes.Repeat([]byte{'x'}, 512))
	return buf.Bytes()
}
