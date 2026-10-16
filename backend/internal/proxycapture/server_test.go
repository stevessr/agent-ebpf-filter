package proxycapture

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func TestOptInAndPortValidation(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Start("I_UNDERSTAND_PROXY_CAPTURE"); err == nil {
		t.Fatal("must not start without user configured ports")
	}
	if err := m.Configure(Config{HTTPPort:8081, SOCKS5Port:8081}); err == nil {
		t.Fatal("same ports must not be accepted")
	}
	if err := m.Configure(Config{HTTPPort:80}); err == nil {
		t.Fatal("privileged ports must not be accepted")
	}
	if err := m.Configure(Config{HTTPPort:freeTCPPort(t)}); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(""); err == nil {
		t.Fatal("start must require acknowledged consent")
	}
	if m.Status().Running {
		t.Fatal("capture must be disabled by default")
	}
	if err := m.Start("I_UNDERSTAND_PROXY_CAPTURE"); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if err := m.Configure(Config{HTTPPort:freeTCPPort(t)}); err == nil {
		t.Fatal("cannot reconfigure an active proxy")
	}
	m.Stop()
	if m.Status().Running { t.Fatal("stop did not shut down listener") }
}

func TestSOCKSOnlyTLSConfiguration(t *testing.T) {
	cfg := Config{SOCKS5Port: 8900, InterceptTLS: true, TLSAllowlist: "api.example.com"}
	if err := validConfig(cfg); err != nil {
		t.Fatalf("SOCKS-only HTTPS capture settings rejected: %v", err)
	}
	cfg.TLSAllowlist = ""
	if err := validConfig(cfg); err == nil {
		t.Fatal("TLS interception without an explicit allowlist must fail")
	}
}

func TestGenerateCAHasNoImplicitTrust(t *testing.T) {
	dir:=t.TempDir()
	m:=NewManager(dir)
	if m.Status().CAReady {t.Fatal("CA must not be generated implicitly")}
	if err:=m.GenerateCA();err!=nil{t.Fatal(err)}
	if !m.Status().CAReady || len(m.Status().CAFingerprint)!=64 {t.Fatal("generated CA fingerprint missing")}
	if _,err:=m.CA();err!=nil{t.Fatal(err)}
	if err:=m.GenerateCA();err==nil{t.Fatal("existing CA must never be overwritten")}
	if _,err:=os.Stat(filepath.Join(dir,"capture-ca-key.pem"));err!=nil{t.Fatal(err)}
}

func TestHTTPProxyCapturesSanitizedJSONWithoutURLQuery(t *testing.T) {
	upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		if r.URL.Query().Get("token")!="sensitive" {t.Error("upstream query lost")}
		if r.Method!="POST"{t.Error("method lost")}
		w.Header().Set("Content-Type","application/json")
		_,_=io.WriteString(w,"{\"ok\":true,\"password\":\"hidden\"}")
	}))
	defer upstream.Close()
	port:=freeTCPPort(t)
	m:=NewManager(t.TempDir())
	if err:=m.Configure(Config{HTTPPort:port,JSONPreview:true});err!=nil{t.Fatal(err)}
	if err:=m.Start("I_UNDERSTAND_PROXY_CAPTURE");err!=nil{t.Fatal(err)}
	defer m.Stop()
	proxyURL,_:=url.Parse("http://"+net.JoinHostPort("127.0.0.1",strconv.Itoa(port)))
	client:=&http.Client{Transport:&http.Transport{Proxy:http.ProxyURL(proxyURL)},Timeout:5*time.Second}
	resp,err:=client.Post(upstream.URL+"/submit?token=sensitive","application/json",strings.NewReader("{\"text\":\"hello\",\"access_token\":\"private\"}"))
	if err!=nil{t.Fatal(err)}
	_,_=io.Copy(io.Discard,resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode!=200{t.Fatalf("unexpected status %d",resp.StatusCode)}
	flows:=m.Flows()
	if len(flows)!=1{t.Fatalf("want one flow, got %d",len(flows))}
	flow:=flows[0]
	if flow.Path!="/submit"{t.Fatalf("query leaked in path: %s",flow.Path)}
	if strings.Contains(flow.RequestJSON,"private") || strings.Contains(flow.ResponseJSON,"hidden") {
		t.Fatalf("sensitive JSON fields were not scrubbed: %+v",flow)
	}
	if !strings.Contains(flow.RequestJSON,"hello") || !strings.Contains(flow.ResponseJSON,"true") {
		t.Fatalf("expected contextual JSON previews: %+v",flow)
	}
}

