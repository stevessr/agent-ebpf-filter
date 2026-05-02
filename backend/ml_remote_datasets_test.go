package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

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

func TestPullRemoteDatasetFromHTTPServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"commandLine":"sudo systemctl disable firewalld","label":"ALERT"},
			{"commandLine":"ls -la /tmp","label":"ALLOW"}
		]`))
	}))
	defer srv.Close()

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		URL:       srv.URL,
		Format:    "auto",
		Limit:     10,
		LabelMode: "preserve",
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
	if resp.Rows[0].Label != "ALERT" || resp.Rows[0].Comm != "sudo" {
		t.Fatalf("first row = %#v", resp.Rows[0])
	}
}

func TestPullRemoteDatasetRejectsHTMLLandingPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang=en dir=ltr prefix=content: http://purl.org/rss/1.0/modules/content/ dc: http://purl.org/dc/terms/>
<head><title>Dataset</title></head>
<body>Download page</body>
</html>`))
	}))
	defer srv.Close()

	_, err := pullRemoteDataset(remoteDatasetRequest{
		URL:    srv.URL,
		Format: "auto",
		Limit:  10,
	})
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
	sample := buildRemoteDatasetSample(row, "block")
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
