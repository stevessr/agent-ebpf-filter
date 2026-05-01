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

	records, format, err := parseRemoteDatasetRecords(raw, "auto")
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
	// GTFOBins style
	gtfoRaw := []byte(`{
		"7z": {
			"functions": {
				"file-read": [
					{ "code": "7z a -ttar -an -so /etc/shadow | 7z e -ttar -si -so" }
				]
			}
		}
	}`)
	records, _, err := parseRemoteDatasetRecords(gtfoRaw, "auto")
	if err != nil {
		t.Fatalf("GTFOBins parse error = %v", err)
	}
	if len(records) != 1 || records[0].Comm != "7z" || records[0].Category != "file-read" {
		t.Fatalf("GTFOBins record = %#v", records[0])
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
	records, _, err = parseRemoteDatasetRecords(lolbasRaw, "auto")
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
	records, _, err := parseRemoteDatasetRecords(raw, "auto")
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
