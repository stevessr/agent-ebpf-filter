package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAntigravityOverrideAndMissing(t *testing.T) {
	t.Setenv("AGENT_RENEW_ANTIGRAVITY_CONFIG","relative.json")
	if _,err:=antigravityConfigPath();err==nil {t.Fatal("relative Antigravity path must be rejected")}
	path:=filepath.Join(t.TempDir(),"gui_config.json")
	t.Setenv("AGENT_RENEW_ANTIGRAVITY_CONFIG",path)
	got,err:=antigravityConfigPath()
	if err!=nil || got!=path {t.Fatalf("Antigravity path %s %v",got,err)}
	s:=loadAntigravitySnapshot(context.Background(),path)
	if s.Found || s.Err!="" {t.Fatalf("missing manager should be optional: %+v",s)}
	if _,err:=os.Stat(path);!os.IsNotExist(err) {t.Fatalf("read-only access created config: %v",err)}
}

func TestAntigravityPointerIsReadOnly(t *testing.T) {
	if runtime.GOOS=="windows" {t.Skip("HOME override not portable on Windows")}
	home:=t.TempDir()
	t.Setenv("HOME",home)
	t.Setenv("ABV_DATA_DIR","")
	t.Setenv("AGENT_RENEW_ANTIGRAVITY_CONFIG","")
	t.Setenv("XDG_CONFIG_HOME",filepath.Join(home,".config"))
	dir:=filepath.Join(home,"relocated")
	pointer:=filepath.Join(home,".antigravity_tools_location")
	if err:=os.WriteFile(pointer,[]byte(dir),0600);err!=nil{t.Fatal(err)}
	path,err:=antigravityConfigPath()
	if err!=nil || path!=filepath.Join(dir,"gui_config.json"){t.Fatalf("pointer resolution %q %v",path,err)}
	if _,err:=os.Stat(dir);!os.IsNotExist(err){t.Fatalf("read-only pointer resolution created relocated data dir: %v",err)}
}

func TestAntigravityConfigAndRequestUsageExcludeAllSecrets(t *testing.T) {
	if _,err:=exec.LookPath("sqlite3");err!=nil{t.Skip("sqlite3 unavailable")}
	dir:=t.TempDir()
	path:=filepath.Join(dir,"gui_config.json")
	secret:="DO_NOT_LEAK_PROXY_ADMIN_KEY_2026"
	config:=`{"proxy":{"enabled":true,"port":8787,"allow_lan_access":false,"auth_mode":"strict","api_key":"`+secret+`","admin_password":"`+secret+`","enable_logging":true,"auto_start":true,"custom_mapping":{"m":"`+secret+`"}},"scheduled_warmup":{"enabled":true},"quota_protection":{"enabled":true,"threshold_percentage":20},"circuit_breaker":{"enabled":true}}`
	if err:=os.WriteFile(path,[]byte(config),0600);err!=nil{t.Fatal(err)}
	db:=filepath.Join(dir,"proxy_logs.db")
	sql:="CREATE TABLE request_logs (timestamp INTEGER, status INTEGER, input_tokens INTEGER, output_tokens INTEGER, request_body TEXT, request_headers TEXT);" +
		"INSERT INTO request_logs VALUES (CAST(strftime('%s','now') AS INTEGER)*1000,200,123,43,'"+secret+"','"+secret+"');" +
		"INSERT INTO request_logs VALUES (CAST(strftime('%s','now') AS INTEGER)*1000,503,10,1,'"+secret+"','"+secret+"');" +
		"INSERT INTO request_logs VALUES (0,200,999,999,'older','older');"
	if out,err:=exec.Command("sqlite3",db,sql).CombinedOutput();err!=nil{t.Fatalf("seed logs %v: %s",err,out)}
	ctx,cancel:=context.WithTimeout(context.Background(),4*time.Second)
	defer cancel()
	s:=loadAntigravitySnapshot(ctx,path)
	if !s.Found||s.Err!=""||!s.Settings.Proxy.Enabled||s.Settings.Proxy.Port!=8787||
		!s.Settings.QuotaProtection.Enabled||s.Settings.QuotaProtection.Threshold!=20||
		!s.UsageReady||s.Usage.Requests!=2||s.Usage.Failures!=1||
		s.Usage.InputTokens!=133||s.Usage.OutputTokens!=44{
		t.Fatalf("Antigravity projection mismatch: %+v",s)
	}
	wire,_:=json.Marshal(s)
	if strings.Contains(string(wire),secret){t.Fatal("Antigravity snapshot retained a credential")}
}

func TestAntigravityUnsafeConfigRejected(t *testing.T) {
	dir:=t.TempDir()
	path:=filepath.Join(dir,"gui_config.json")
	if err:=os.WriteFile(path,[]byte("{"),0600);err!=nil{t.Fatal(err)}
	s:=loadAntigravitySnapshot(context.Background(),path)
	if !s.Found||s.Err==""{t.Fatalf("expected malformed config error: %+v",s)}
}

func TestOtherManagerProcessIdentities(t *testing.T) {
	for _,tc:=range []struct{comm,want string}{
		{"ccr","ccr"},
		{"claude-code-rou","ccr"},
		{"claude-code-router","ccr"},
		{"antigravity-too","antigravity"},
		{"antigravity-tools","antigravity"},
		{"AstrLink","astrlink"},
		{"node",""},
		{"bash",""},
	}{
		got:=managementAppForEvent(eventSummary{Comm:tc.comm})
		if got!=tc.want{t.Errorf("process %q = %q, want %q",tc.comm,got,tc.want)}
	}
}
