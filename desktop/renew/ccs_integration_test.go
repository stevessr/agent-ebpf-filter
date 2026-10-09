package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCCSAppForEventMatchesOnlyExecutable(t *testing.T) {
	cases := []struct {
		comm string
		want string
	}{
		{"claude", "claude"},
		{"Claude.EXE", "claude"},
		{"codex", "codex"},
		{"gemini", "gemini"},
		{"grokbuild", "grokbuild"},
		{"opencode", "opencode"},
		{"pi", "pi"},
		{"bash", ""},
		{"node", ""},
		{"codex-helper", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := ccsAppForEvent(eventSummary{Comm: tc.comm})
		if got != tc.want {
			t.Errorf("ccsAppForEvent(%q) = %q, want %q", tc.comm, got, tc.want)
		}
	}
}

func TestCCSDatabasePathOverride(t *testing.T) {
	t.Setenv("AGENT_RENEW_CCS_DB", "relative.db")
	if _, err := ccsDatabasePath(); err == nil {
		t.Fatal("relative override must be rejected")
	}
	path := filepath.Join(t.TempDir(), "cc-switch.db")
	t.Setenv("AGENT_RENEW_CCS_DB", path)
	got, err := ccsDatabasePath()
	if err != nil || got != path {
		t.Fatalf("override = %q, err=%v; want %q", got, err, path)
	}
}

func TestLoadCCSAbsentDoesNotCreateDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.db")
	snapshot := loadCCSSnapshot(context.Background(), path)
	if snapshot.Found || snapshot.Err != "" {
		t.Fatalf("missing CCS database should be optional, got %+v", snapshot)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("read-only scan created a database: %v", err)
	}
}

func TestLoadCCSSnapshotQueriesOnlySafeMetadata(t *testing.T) {
	bin, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("optional sqlite3 CLI is not installed")
	}
	path := filepath.Join(t.TempDir(), "cc-switch.db")
	sql := "CREATE TABLE providers (app_type TEXT, name TEXT, is_current INTEGER, settings_config TEXT);" +
		"CREATE TABLE proxy_config (app_type TEXT, listen_address TEXT, listen_port INTEGER, enabled INTEGER, proxy_enabled INTEGER);" +
		"CREATE TABLE proxy_request_logs (app_type TEXT, status_code INTEGER, input_tokens INTEGER, output_tokens INTEGER, latency_ms INTEGER, created_at INTEGER, data_source TEXT);" +
		"INSERT INTO providers VALUES ('codex','Example',1,'SENSITIVE_API_KEY_MUST_NOT_LEAK');" +
		"INSERT INTO providers VALUES ('gemini','Unused',0,'OTHER_SECRET');" +
		"INSERT INTO proxy_config VALUES ('codex','127.0.0.1',15721,1,1);" +
		"INSERT INTO proxy_request_logs VALUES ('codex',200,400,120,300,CAST(strftime('%s','now') AS INTEGER),'proxy');" +
		"INSERT INTO proxy_request_logs VALUES ('codex',503,100,0,700,CAST(strftime('%s','now') AS INTEGER),'proxy');"
	if out, err := exec.Command(bin, path, sql).CombinedOutput(); err != nil {
		t.Fatalf("seed db: %v: %s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	snapshot := loadCCSSnapshot(ctx, path)
	if !snapshot.Found || snapshot.Err != "" {
		t.Fatalf("CCS db should load: %+v", snapshot)
	}
	if len(snapshot.Providers) != 1 || snapshot.Providers[0].Name != "Example" || snapshot.Providers[0].App != "codex" {
		t.Fatalf("provider metadata mismatch: %+v", snapshot.Providers)
	}
	if len(snapshot.Proxies) != 1 || snapshot.Proxies[0].Port != 15721 {
		t.Fatalf("proxy metadata mismatch: %+v", snapshot.Proxies)
	}
	if len(snapshot.Usage) != 1 || snapshot.Usage[0].Requests != 2 || snapshot.Usage[0].Failures != 1 || snapshot.Usage[0].InputTokens != 500 || snapshot.Usage[0].OutputTokens != 120 || snapshot.Usage[0].AvgLatencyMS != 500 {
		t.Fatalf("usage aggregate mismatch: %+v (err=%q)", snapshot.Usage, snapshot.UsageErr)
	}
	if !snapshot.FetchedAt.After(time.Now().Add(-10 * time.Second)) {
		t.Fatal("metadata refresh timestamp missing")
	}
}
