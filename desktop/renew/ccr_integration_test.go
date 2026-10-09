package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCCRConfigDBOverrideAndMissing(t *testing.T) {
	t.Setenv("AGENT_RENEW_CCR_CONFIG_DB", "relative.sqlite")
	if _, err := ccrConfigPath(); err == nil { t.Fatal("relative CCR DB path must be rejected") }
	path := filepath.Join(t.TempDir(), "config.sqlite")
	t.Setenv("AGENT_RENEW_CCR_CONFIG_DB", path)
	actual, err := ccrConfigPath()
	if err != nil || actual != path { t.Fatalf("CCR DB override %s: %v", actual, err) }
	out := loadCCRSnapshot(context.Background(), path)
	if out.Found || out.Err != "" { t.Fatalf("CCR absence is optional: %+v", out) }
	if _, err := os.Stat(path); !os.IsNotExist(err) { t.Fatalf("read-only must never create DB: %v", err) }
}

func TestCCRReadonlySummaryNeverRetainsCredentials(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil { t.Skip("sqlite3 unavailable") }
	dir := t.TempDir()
	db := filepath.Join(dir, "config.sqlite")
	secret := "SECRET_CCR_API_KEY_MUST_NOT_LEAK"
	payload := `{"Providers":[{"name":"Sandbox","apiKey":"`+secret+`"},{"name":"Another"}],"preferredProvider":"Sandbox","PORT":3456,"APIKEY":"`+secret+`"}`
	escaped := strings.ReplaceAll(payload, "'", "''")
	sql := "CREATE TABLE app_config (key TEXT PRIMARY KEY,value_json TEXT NOT NULL);" +
		"INSERT INTO app_config VALUES ('default','"+escaped+"');" +
		"CREATE TABLE api_keys (id TEXT,encrypted_key TEXT);" +
		"INSERT INTO api_keys VALUES ('1','"+secret+"');"
	if out, err := exec.Command("sqlite3",db,sql).CombinedOutput(); err != nil { t.Fatalf("seed CCR config: %v: %s",err,out) }
	usage := filepath.Join(dir,"app-data","usage.sqlite")
	if err := os.MkdirAll(filepath.Dir(usage),0700); err != nil { t.Fatal(err) }
	usageSQL := "CREATE TABLE usage_events (created_at TEXT,status_code INTEGER,input_tokens INTEGER,output_tokens INTEGER,request_id TEXT,credential_id TEXT);" +
		"INSERT INTO usage_events VALUES (strftime('%Y-%m-%dT%H:%M:%SZ','now'),200,100,50,'SECRET_REQUEST_ID','SECRET_CREDENTIAL');" +
		"INSERT INTO usage_events VALUES (strftime('%Y-%m-%dT%H:%M:%SZ','now'),502,25,0,'SECRET_REQUEST_ID_2','SECRET_CREDENTIAL');" +
		"INSERT INTO usage_events VALUES ('2000-01-01T00:00:00Z',200,9999,9999,'older','older');"
	if out, err := exec.Command("sqlite3",usage,usageSQL).CombinedOutput(); err != nil { t.Fatalf("seed CCR usage: %v: %s",err,out) }
	ctx, cancel := context.WithTimeout(context.Background(),5*time.Second)
	defer cancel()
	s := loadCCRSnapshot(ctx,db)
	if !s.Found || s.Err != "" || s.Config.Providers != 2 || s.Config.PreferredProvider != "Sandbox" ||
		s.Config.ConfiguredPort != 3456 || !s.UsageReady || s.Usage.Requests != 2 ||
		s.Usage.Failures != 1 || s.Usage.InputTokens != 125 || s.Usage.OutputTokens != 50 {
		t.Fatalf("CCR metadata projection mismatch: %+v",s)
	}
	wire,_ := json.Marshal(s)
	for _, blocked := range []string{secret,"SECRET_REQUEST_ID","SECRET_CREDENTIAL"} {
		if strings.Contains(string(wire),blocked) { t.Errorf("CCR snapshot leaked private material: %s",blocked) }
	}
}

func TestManagementSQLiteRejectsNonSelectAndSymlink(t *testing.T) {
	ctx := context.Background()
	var rows []struct{ Count int `json:"count"` }
	if err := managementSQLiteQuery(ctx,":memory:","DELETE FROM app_config",&rows); err == nil {
		t.Fatal("management sqlite runner must refuse write SQL")
	}
	dir := t.TempDir()
	target := filepath.Join(dir,"db.sqlite")
	if err := os.WriteFile(target,[]byte("test"),0600); err != nil {t.Fatal(err)}
	link := filepath.Join(dir,"link")
	if err := os.Symlink(target,link); err == nil {
		if _,err := regularGatewayFile(link,100); err == nil { t.Fatal("SQLite symlink should be rejected") }
	}
	if _,err := regularGatewayFile(target,1); err == nil { t.Fatal("oversize file must be rejected") }
}
