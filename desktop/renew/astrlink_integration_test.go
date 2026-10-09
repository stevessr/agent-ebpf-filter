package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAstrLinkSessionPathAndMissing(t *testing.T) {
	t.Setenv("AGENT_RENEW_ASTRLINK_SESSION", "relative.json")
	if _, err := astrLinkSessionPath(); err == nil {
		t.Fatal("relative override must be refused")
	}
	path := filepath.Join(t.TempDir(), "control-session.json")
	t.Setenv("AGENT_RENEW_ASTRLINK_SESSION", path)
	resolved, err := astrLinkSessionPath()
	if err != nil || resolved != path {
		t.Fatalf("session override: %q, %v", resolved, err)
	}
	session, found, err := readAstrLinkSession(path)
	if err != nil || found || session.ControlSocket != "" {
		t.Fatalf("missing AstrLink must be optional: %+v %v %v", session, found, err)
	}
}

func TestAstrLinkSessionRejectsSymlinkAndOversize(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.json")
	if err := os.WriteFile(target, []byte(`{"schema_version":1,"control_socket":"/tmp/x"}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "session.json")
	if err := os.Symlink(target, link); err == nil {
		_, _, got := readAstrLinkSession(link)
		if got == nil { t.Fatal("session symlink must be rejected") }
	}
	tooLarge := filepath.Join(dir, "large.json")
	if err := os.WriteFile(tooLarge, []byte(strings.Repeat("x", 8193)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readAstrLinkSession(tooLarge); err == nil {
		t.Fatal("oversize file must be rejected")
	}
}

func TestAstrLinkObserverRejectsExternalURL(t *testing.T) {
	cases := []string{
		"https://127.0.0.1:8899",
		"http://example.com:8899",
		"http://127.0.0.1:8899/evil",
		"http://user:password@127.0.0.1:8899",
		"http://127.0.0.1:8899?x=1",
	}
	for _, addr := range cases {
		_, _, _, err := astrLinkClient(astrLinkSession{ControlURL:addr, ControlToken:"observer"})
		if err == nil { t.Errorf("accepted non-loopback control endpoint %q", addr) }
	}
}

func TestAstrLinkObserverSafeProjectionViaLoopback(t *testing.T) {
	var mu sync.Mutex
	var called []string
	const observerToken = "observer-only-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet { t.Errorf("unexpected method %s", r.Method) }
		if r.URL.Path != "/control/v1/health" && r.Header.Get("Authorization") != "Bearer "+observerToken {
			t.Errorf("missing observer-only token on %s", r.URL.Path)
		}
		mu.Lock()
		called = append(called, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/control/v1/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/control/v1/services":
			_, _ = w.Write([]byte(`{"items":[{"name":"My provider","kind":"codex_subscription","enabled":true,"http":{"base_url":"https://secret/endpoint","credential_ref":"never-expose-me"},"subscription":{"account_hint":"private@example.com"}}]}`))
		case "/control/v1/policies":
			_, _ = w.Write([]byte(`{"items":[{"name":"Sensitive guard","enabled":true,"detector":"regex","request_action":"redact","response_action":"allow","match":{"secret":"never-leak"}}]}`))
		case "/control/v1/usage-summary":
			if r.URL.Query().Get("bucket") != "hour" || r.URL.Query().Get("time_zone") != "Etc/UTC" {
				t.Errorf("invalid usage query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"totals":{"requests":17,"failed_requests":2,"input_tokens":1300,"output_tokens":400},"by_token":[{"id":"private"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	if !strings.HasPrefix(server.URL, "http://127.0.0.1:") {
		t.Skip("loopback listener not IPv4 on this host")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "control-session.json")
	payload, _ := json.Marshal(astrLinkSession{SchemaVersion:1, ControlURL:server.URL, ControlToken:observerToken})
	if err := os.WriteFile(path,payload,0600); err != nil { t.Fatal(err) }

	result := loadAstrLinkSnapshot(context.Background(), path, time.Now())
	if result.Err != "" || !result.Connected || !result.UsageReady ||
		result.Usage.Requests != 17 || result.Usage.FailedRequests != 2 {
		t.Fatalf("AstrLink observer projection: %+v", result)
	}
	if len(result.Providers) != 1 || len(result.Policies) != 1 ||
		result.Providers[0].Name != "My provider" || !result.Policies[0].Enabled {
		t.Fatalf("AstrLink data mismatch: %+v", result)
	}
	asJSON, _ := json.Marshal(result)
	for _, secret := range []string{observerToken,"never-expose-me","private@example.com","never-leak","private"} {
		if strings.Contains(string(asJSON),secret) {
			t.Errorf("observer state retained private field %q", secret)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(called) != 4 {
		t.Fatalf("expected health+3 safe reads, got %v", called)
	}
	for _, route := range called {
		if strings.Contains(route,"audit") || strings.Contains(route,"requests") || strings.Contains(route,"tokens") {
			t.Fatalf("unsafe endpoint queried: %s", route)
		}
	}
}

func TestAstrLinkUnixSocketObserver(t *testing.T) {
	if runtime.GOOS == "windows" { t.Skip("Windows uses loopback Observer token") }
	path := filepath.Join(t.TempDir(), "control.sock")
	listener, err := net.Listen("unix",path)
	if err != nil { t.Skipf("unix listener unavailable: %v",err) }
	defer listener.Close()
	server := &http.Server{Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
		if r.Header.Get("Authorization") != "" { t.Error("unix socket must never send bearer credentials") }
		if r.URL.Path == "/control/v1/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`)); return
		}
		_, _ = w.Write([]byte(`{"items":[],"totals":{"requests":0}}`))
	})}
	defer server.Close()
	go func(){ _ = server.Serve(listener) }()
	locator := filepath.Join(t.TempDir(),"control-session.json")
	body := `{"schema_version":1,"control_socket":`+strconvQuote(path)+`,"control_token":"SHOULD_IGNORE"}`
	if err := os.WriteFile(locator, []byte(body),0600); err != nil { t.Fatal(err) }
	result := loadAstrLinkSnapshot(context.Background(),locator,time.Now())
	if !result.Connected || result.Err != "" || !result.UsageReady {
		t.Fatalf("unix observer: %+v",result)
	}
}
func strconvQuote(s string) string { raw, _ := json.Marshal(s); return string(raw) }
