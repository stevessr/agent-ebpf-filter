package app

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"testing"
)

func TestWritePCAPExportFilesCreatesPrivateUniqueArtifacts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	flows := []NetworkFlowSummary{{
		Protocol: "TCP", SrcIP: "10.0.0.1", SrcPort: 1234, DstIP: "203.0.113.1", DstPort: 443,
		DstDomain: "quoted\"domain", IPScope: "public", ProcessComms: []string{"codex\nworker"}, BytesOut: 42,
	}}

	const workers = 12
	type result struct {
		pcap  string
		jsonl string
		err   error
	}
	results := make(chan result, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pcapPath, jsonlPath, _, err := writePCAPExportFiles(dir, flows, nil)
			results <- result{pcap: pcapPath, jsonl: jsonlPath, err: err}
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[string]struct{}, workers)
	for item := range results {
		if item.err != nil {
			t.Fatalf("writePCAPExportFiles() error = %v", item.err)
		}
		if _, exists := seen[item.pcap]; exists {
			t.Fatalf("duplicate PCAP export path %q", item.pcap)
		}
		seen[item.pcap] = struct{}{}
		assertPrivateRegularFile(t, item.pcap)
		assertPrivateRegularFile(t, item.jsonl)
	}

	var oneJSONL string
	for path := range seen {
		oneJSONL = path + ".jsonl"
		break
	}
	file, err := os.Open(oneJSONL)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()
	var payload map[string]any
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&payload); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if payload["dstDomain"] != "quoted\"domain" || payload["comm"] != "codex\nworker" {
		t.Fatalf("sidecar payload = %#v", payload)
	}
}

func assertPrivateRegularFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("%q mode = %s, want regular file", path, info.Mode())
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("%q permissions = %o, want 600", path, got)
	}
}
