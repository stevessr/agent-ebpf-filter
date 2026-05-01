package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
