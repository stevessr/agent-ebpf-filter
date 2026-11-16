// Package proxycapture implements a deliberately local, opt-in HTTP/SOCKS
// debugging proxy. It never alters OS proxy settings or certificate trust.
package proxycapture

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agent-ebpf-filter/internal/domainforwardproxy"
)

const MaxFlows = 300
const MaxJSONPreview = 4096

type Config struct {
	HTTPPort       int    `json:"httpPort"`
	SOCKS5Port     int    `json:"socks5Port"`
	SOCKSPort      int    `json:"socksPort"` // SOCKS4 / SOCKS4a
	InterceptTLS   bool   `json:"interceptTLS"`
	TLSAllowlist   string `json:"tlsAllowlist"`
	JSONPreview    bool   `json:"jsonPreview"`
}

type Flow struct {
	ID            uint64    `json:"id"`
	At            time.Time `json:"at"`
	Protocol      string    `json:"protocol"`
	Method        string    `json:"method,omitempty"`
	Host          string    `json:"host"`
	Path          string    `json:"path,omitempty"` // never includes query strings
	Status        int       `json:"status,omitempty"`
	DurationMS    int64     `json:"durationMs"`
	ContentType   string    `json:"contentType,omitempty"`
	RequestJSON   string    `json:"requestJson,omitempty"`
	ResponseJSON  string    `json:"responseJson,omitempty"`
	Error         string    `json:"error,omitempty"`
}

type Status struct {
	Config       Config  `json:"config"`
	Configured   bool    `json:"configured"`
	Running      bool    `json:"running"`
	CAReady      bool    `json:"caReady"`
	CAFingerprint string `json:"caFingerprint,omitempty"`
	FlowCount    int     `json:"flowCount"`
}

type Manager struct {
	mu sync.Mutex
	config Config
	configured bool
	running bool
	httpServer *http.Server
	listeners []net.Listener
	connections map[net.Conn]struct{}
	flows []Flow
	nextID atomic.Uint64
	caDir string
}

func NewManager(caDir string) *Manager {
	return &Manager{caDir: caDir, connections:make(map[net.Conn]struct{})}
}

func DefaultCADir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return filepath.Join(os.TempDir(), "agent-ebpf-filter", "proxy-capture-ca")
	}
	return filepath.Join(base, "agent-ebpf-filter", "proxy-capture-ca")
}

func validConfig(c Config) error {
	ports := []int{c.HTTPPort, c.SOCKS5Port, c.SOCKSPort}
	seen := map[int]bool{}
	hasPort := false
	for _, port := range ports {
		if port == 0 { continue }
		if port < 1024 || port > 65535 { return errors.New("proxy port must be 1024..65535 (0 disables a protocol)") }
		if seen[port] { return errors.New("HTTP, SOCKS5 and SOCKS4 ports must not conflict") }
		seen[port] = true
		hasPort = true
	}
	if !hasPort { return errors.New("configure at least one HTTP/SOCKS5/SOCKS4 port before starting") }
	if c.InterceptTLS && strings.TrimSpace(c.TLSAllowlist) == "" {
		return errors.New("TLS interception requires an explicit domain allowlist")
	}
	return nil
}

func (m *Manager) Configure(c Config) error {
	if err := validConfig(c); err != nil { return err }
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running { return errors.New("stop capture before changing ports or TLS settings") }
	m.config, m.configured = c, true
	return nil
}

func (m *Manager) caPaths() (string, string) {
	return filepath.Join(m.caDir, "capture-ca.pem"), filepath.Join(m.caDir, "capture-ca-key.pem")
}

func (m *Manager) CA() ([]byte, error) {
	certPath, _ := m.caPaths()
	return os.ReadFile(certPath)
}

// GenerateCA is called only through a separately acknowledged API action.
// The key never appears in the response; no trust store is ever changed here.
func (m *Manager) GenerateCA() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running { return errors.New("stop proxy capture before generating a CA") }
	certPath, keyPath := m.caPaths()
	if _, err := os.Stat(certPath); err == nil { return errors.New("capture CA already exists; refusing to overwrite trusted identity") }
	if _, err := os.Stat(keyPath); err == nil { return errors.New("capture CA private key already exists") }
	if err := os.MkdirAll(m.caDir, 0700); err != nil { return err }
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil { return err }
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil { return err }
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{CommonName:"Agent eBPF Filter LOCAL Proxy Capture CA", Organization:[]string{"Agent eBPF Filter"}},
		NotBefore: now.Add(-time.Minute),
		NotAfter: now.Add(180*24*time.Hour),
		IsCA:true, BasicConstraintsValid:true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		MaxPathLenZero:true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil { return err }
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil { return err }
	// Create private key exclusively and first, to prevent any readable CA
	// certificate being published before a matching key is safely persisted.
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil { return err }
	_, err = keyFile.Write(pem.EncodeToMemory(&pem.Block{Type:"PRIVATE KEY", Bytes:pkcs8}))
	closeErr := keyFile.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(keyPath)
		return errors.Join(err, closeErr)
	}
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil { _ = os.Remove(keyPath); return err }
	_, err = certFile.Write(pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE", Bytes:der}))
	closeErr = certFile.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(certPath)
		_ = os.Remove(keyPath)
		return errors.Join(err, closeErr)
	}
	return nil
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := Status{Config:m.config, Configured:m.configured, Running:m.running, FlowCount:len(m.flows)}
	if data, err := m.CA(); err == nil {
		if block,_ := pem.Decode(data); block != nil {
			if cert,err := x509.ParseCertificate(block.Bytes); err == nil {
				status.CAReady = true
				hash := sha256.Sum256(cert.Raw)
				status.CAFingerprint = strings.ToUpper(hex.EncodeToString(hash[:]))
			}
		}
	}
	return status
}

func (m *Manager) Flows() []Flow {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Flow, len(m.flows))
	for i:=range m.flows { result[i] = m.flows[len(m.flows)-1-i] }
	return result
}

func (m *Manager) Clear() {
	m.mu.Lock()
	m.flows = nil
	m.mu.Unlock()
}

func (m *Manager) record(f Flow) {
	f.ID = m.nextID.Add(1)
	f.At = time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running { return }
	if len(m.flows) == MaxFlows { copy(m.flows, m.flows[1:]); m.flows[len(m.flows)-1] = f } else { m.flows = append(m.flows,f) }
}

func (m *Manager) Start(ack string) error {
	if ack != "I_UNDERSTAND_PROXY_CAPTURE" { return errors.New("explicit proxy capture consent required") }
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running { return errors.New("proxy capture already running") }
	if !m.configured { return errors.New("configure ports before enabling capture") }
	if err := validConfig(m.config); err != nil { return err }
	cfg := m.config
	if cfg.InterceptTLS {
		certPath, keyPath := m.caPaths()
		if _, err := domainforwardproxy.NewCaptureTLSConfig(certPath, keyPath, cfg.TLSAllowlist, firstAllowedHost(cfg.TLSAllowlist)); err != nil { return fmt.Errorf("capture CA not ready: %w", err) }
	}
	type listener struct { name string; l net.Listener }
	open := []listener{}
	for _, item := range []struct {name string; port int}{{"http",cfg.HTTPPort},{"socks5",cfg.SOCKS5Port},{"socks",cfg.SOCKSPort}} {
		if item.port==0 { continue }
		l,err := net.Listen("tcp",net.JoinHostPort("127.0.0.1",strconv.Itoa(item.port)))
		if err != nil { for _, prev := range open { _ = prev.l.Close() }; return fmt.Errorf("%s port: %w",item.name,err) }
		open=append(open,listener{item.name,l})
	}
	m.running=true
	m.listeners=nil
	for _, item := range open {
		m.listeners=append(m.listeners,item.l)
		switch item.name {
		case "http":
			srv:= &http.Server{Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){m.httpRequest(w,r,cfg,false,"")}), ReadHeaderTimeout:10*time.Second, IdleTimeout:30*time.Second}
			m.httpServer=srv
			go func(l net.Listener){_ = srv.Serve(l)}(item.l)
		default:
			go m.serveSOCKS(item.l,item.name)
		}
	}
	return nil
}

func firstAllowedHost(s string) string {
	for _, item := range strings.FieldsFunc(s,func(r rune)bool{return r==',' || r=='\n' || r==';'}) {
		item=strings.TrimSpace(item)
		if item!="" {
			if strings.HasPrefix(item,"*.") { return "capture."+strings.TrimPrefix(item,"*.") }
			return item
		}
	}
	return ""
}

func (m *Manager) Stop() {
	m.mu.Lock()
	servers, listeners := m.httpServer, m.listeners
	connections := make([]net.Conn,0,len(m.connections))
	for conn := range m.connections { connections=append(connections,conn) }
	m.httpServer=nil
	m.listeners=nil
	m.running=false
	m.mu.Unlock()
	for _, conn := range connections { _=conn.Close() }
	for _, l := range listeners { _=l.Close() }
	if servers!=nil { _=servers.Close() }
}

func (m *Manager) track(conn net.Conn) bool {
 m.mu.Lock()
 defer m.mu.Unlock()
 if !m.running {return false}
 m.connections[conn]=struct{}{}
 return true
}
func (m *Manager) untrack(conn net.Conn) {
 m.mu.Lock()
 delete(m.connections,conn)
 m.mu.Unlock()
}

func destination(addr, defaultPort string)(string,string,error) {
	if !strings.Contains(addr,":") { addr=net.JoinHostPort(addr,defaultPort) }
	host,port,err := net.SplitHostPort(addr)
	if err!=nil || host=="" { return "","",errors.New("invalid proxy destination") }
	n,err:=strconv.Atoi(port)
	if err!=nil || n<1 || n>65535 { return "","",errors.New("invalid destination port") }
	return net.JoinHostPort(host,port),strings.Trim(host,"[]"),nil
}

func (m *Manager) httpRequest(w http.ResponseWriter,r *http.Request,cfg Config,tlsIntercept bool,connectAddr string) {
	start:=time.Now()
	if r.Method==http.MethodConnect && !tlsIntercept {
		m.connect(w,r,cfg)
		return
	}
	if r.Method==http.MethodConnect { http.Error(w,"nested CONNECT forbidden",http.StatusBadRequest); return }
	u := *r.URL
	if tlsIntercept {
		u.Scheme="https"; u.Host=connectAddr
	} else if u.Host=="" { u.Host=r.Host }
	if u.Scheme=="" { u.Scheme="http" }
	if u.Scheme!="http" && u.Scheme!="https" {http.Error(w,"unsupported scheme",400);return}
	_,host,err:=destination(u.Host,map[bool]string{true:"443",false:"80"}[u.Scheme=="https"])
	if err!=nil {http.Error(w,err.Error(),400);return}
	out:=r.Clone(r.Context())
	out.URL=&u
	out.RequestURI=""
	out.Host=u.Host
	out.Header=r.Header.Clone()
	cleanHop(out.Header)
	var requestPreview string
	if cfg.JSONPreview { requestPreview=sampleJSON(&out.Body,out.Header.Get("Content-Type"),out.ContentLength) }
	tr:=http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy=nil // do not recursively use system proxy
	tr.TLSHandshakeTimeout=10*time.Second
	tr.ResponseHeaderTimeout=30*time.Second
	defer tr.CloseIdleConnections()
	res,err:=tr.RoundTrip(out)
	flow:=Flow{Protocol:map[bool]string{true:"HTTPS MITM",false:"HTTP"}[tlsIntercept],Method:r.Method,Host:host,Path:u.EscapedPath(),RequestJSON:requestPreview}
	if err!=nil {
		flow.Error="upstream request failed"
		flow.DurationMS=time.Since(start).Milliseconds()
		m.record(flow)
		http.Error(w,"upstream request failed",http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	flow.Status=res.StatusCode
	flow.ContentType=res.Header.Get("Content-Type")
	cleanHop(res.Header)
	var responsePreview string
	if cfg.JSONPreview {responsePreview=sampleJSON(&res.Body,res.Header.Get("Content-Type"),res.ContentLength)}
	flow.ResponseJSON=responsePreview
	for k,vs:=range res.Header {for _,v:=range vs{w.Header().Add(k,v)}}
	w.WriteHeader(res.StatusCode)
	_,_ = io.Copy(w,res.Body)
	flow.DurationMS=time.Since(start).Milliseconds()
	m.record(flow)
}

func cleanHop(h http.Header) {
	for _, item:=range strings.Split(h.Get("Connection"),","){ h.Del(strings.TrimSpace(item)) }
	for _, key:=range []string{"Connection","Keep-Alive","Proxy-Authenticate","Proxy-Authorization","Proxy-Connection","TE","Trailer","Transfer-Encoding","Upgrade"} { h.Del(key) }
}

func sampleJSON(body *io.ReadCloser, contentType string, size int64) string {
	if size<0 || size>MaxJSONPreview || !strings.Contains(strings.ToLower(contentType),"application/json") || *body==nil {return ""}
	data,err:=io.ReadAll(io.LimitReader(*body,MaxJSONPreview+1))
	if err!=nil {return ""}
	*body=io.NopCloser(io.MultiReader(bytes.NewReader(data),*body))
	if len(data)>MaxJSONPreview {return ""}
	var obj any
	if json.Unmarshal(data,&obj)!=nil {return ""}
	sanitizeJSON(obj)
	clean,err:=json.Marshal(obj)
	if err!=nil {return ""}
	return string(clean)
}

func sanitizeJSON(v any) {
	switch x:=v.(type) {
	case map[string]any:
		for key,value:=range x {
			k:=strings.ToLower(key)
			sensitive:=false
			for _,part:=range []string{"token","secret","password","cookie","credential","authorization","api_key","apikey","session","private","access_key","bearer"} {
				if strings.Contains(k,part) {sensitive=true;break}
			}
			if sensitive { x[key]="[REDACTED]" } else {sanitizeJSON(value)}
		}
	case []any: for _,item:=range x {sanitizeJSON(item)}
	}
}

func (m *Manager) connect(w http.ResponseWriter,r *http.Request,cfg Config) {
	start:=time.Now()
	address,host,err:=destination(r.Host,"443")
	if err!=nil {http.Error(w,err.Error(),400);return}
	hijacker,ok:=w.(http.Hijacker)
	if !ok {http.Error(w,"proxy cannot hijack this HTTP connection",500);return}
	client,rw,err:=hijacker.Hijack()
	if err!=nil {http.Error(w,"proxy cannot hijack",500);return}
	defer client.Close()
	if !m.track(client) {return}
	defer m.untrack(client)
	// CONNECT is acknowledged only once we can either terminate TLS ourselves
	// or have successfully established the upstream TCP connection.
	if cfg.InterceptTLS && allowedHost(cfg.TLSAllowlist,host) {
		certPath,keyPath:=m.caPaths()
		tlsCfg,caErr:=domainforwardproxy.NewCaptureTLSConfig(certPath,keyPath,cfg.TLSAllowlist,host)
		if caErr==nil {
			_,_ = io.WriteString(client,"HTTP/1.1 200 Connection Established\r\n\r\n")
			var tlsBase net.Conn=client
			if rw.Reader.Buffered()>0 {tlsBase=&bufferedConn{Conn:client,reader:rw.Reader}}
			tlsConn:=tls.Server(tlsBase,tlsCfg)
			defer tlsConn.Close()
			if err:=tlsConn.Handshake();err!=nil {
				m.record(Flow{Protocol:"HTTPS MITM",Host:host,Error:"TLS handshake failed"})
				return
			}
			srv:=&http.Server{Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){m.httpRequest(w,r,cfg,true,address)}),ReadHeaderTimeout:15*time.Second}
			_ = srv.Serve(&oneConnListener{conn:tlsConn})
			return
		}
		m.record(Flow{Protocol:"HTTPS MITM",Host:host,Error:"CA unavailable"})
		_,_=io.WriteString(client,"HTTP/1.1 502 Bad Gateway\r\n\r\n")
		return
	}
	upstream,err:=net.DialTimeout("tcp",address,10*time.Second)
	if err!=nil {_,_=io.WriteString(client,"HTTP/1.1 502 Bad Gateway\r\n\r\n");return}
	defer upstream.Close()
	_,_=io.WriteString(client,"HTTP/1.1 200 Connection Established\r\n\r\n")
	// Hijacked buffered reader may hold initial TLS bytes already received.
	stream:=net.Conn(client)
	if rw.Reader.Buffered()>0 {stream=&bufferedConn{Conn:client, reader:rw.Reader}}
	pump(stream,upstream)
	m.record(Flow{Protocol:"HTTPS CONNECT",Host:host,DurationMS:time.Since(start).Milliseconds()})
}

type bufferedConn struct {net.Conn;reader *bufio.Reader}
func (c *bufferedConn) Read(p []byte)(int,error){return c.reader.Read(p)}

type oneConnListener struct {mu sync.Mutex;conn net.Conn;used bool;done chan struct{}}
type closeNotifyingConn struct {net.Conn;done chan struct{};once sync.Once}
func (c *closeNotifyingConn) Close() error {c.once.Do(func(){close(c.done)});return c.Conn.Close()}
func (l *oneConnListener) Accept()(net.Conn,error){
 l.mu.Lock()
 if !l.used {
  l.used=true
  l.done=make(chan struct{})
  conn:=&closeNotifyingConn{Conn:l.conn,done:l.done}
  l.mu.Unlock()
  return conn,nil
 }
 done:=l.done
 l.mu.Unlock()
 <-done
 return nil,io.EOF
}
func (l *oneConnListener) Close() error {return nil}
func (l *oneConnListener) Addr() net.Addr {return l.conn.LocalAddr()}

func pump(client, upstream net.Conn) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func(){defer wg.Done();_,_=io.Copy(upstream,client);if c,ok:=upstream.(*net.TCPConn);ok{_ = c.CloseWrite()}}()
	_,_=io.Copy(client,upstream)
	if c,ok:=client.(*net.TCPConn);ok{_ = c.CloseWrite()}
	wg.Wait()
}

func allowedHost(list,host string) bool {
	host=strings.ToLower(strings.TrimSuffix(host,"."))
	for _,raw:=range strings.FieldsFunc(list,func(r rune)bool{return r==',' || r==';' || r=='\n'}) {
		pattern:=strings.ToLower(strings.TrimSpace(raw))
		if pattern==host {return true}
		if strings.HasPrefix(pattern,"*.") && strings.HasSuffix(host,strings.TrimPrefix(pattern,"*")) && host!=strings.TrimPrefix(pattern,"*.") {return true}
	}
	return false
}

func (m *Manager) serveSOCKS(l net.Listener, proto string) {
	for {
		conn,err:=l.Accept()
		if err!=nil {return}
		go m.handleSOCKS(conn,proto)
	}
}

func (m *Manager) handleSOCKS(conn net.Conn,proto string) {
	defer conn.Close()
	if !m.track(conn) {return}
	defer m.untrack(conn)
	_ = conn.SetDeadline(time.Now().Add(10*time.Second))
	reader:=bufio.NewReaderSize(conn,512)
	var address,host string
	var err error
	switch proto {
	case "socks5": address,host,err=socks5Handshake(conn,reader)
	case "socks": address,host,err=socks4Handshake(conn,reader)
	}
	if err!=nil {return}
	start:=time.Now()
	m.mu.Lock()
	cfg:=m.config
	m.mu.Unlock()
	// SOCKS CONNECT can also carry HTTPS. Only terminate TLS when the user
	// explicitly enabled interception for this exact destination.
	if cfg.InterceptTLS && strings.HasSuffix(address,":443") && allowedHost(cfg.TLSAllowlist,host) {
		certPath,keyPath:=m.caPaths()
		tlsConfig,caErr:=domainforwardproxy.NewCaptureTLSConfig(certPath,keyPath,cfg.TLSAllowlist,host)
		if caErr != nil {return}
		if proto=="socks5" {
			_,_ = conn.Write([]byte{5,0,0,1,0,0,0,0,0,0})
		} else {
			_,_ = conn.Write([]byte{0,90,0,0,0,0,0,0})
		}
		var stream net.Conn=conn
		if reader.Buffered()>0 {stream=&bufferedConn{Conn:conn,reader:reader}}
		tlsConn:=tls.Server(stream,tlsConfig)
		defer tlsConn.Close()
		if err:=tlsConn.Handshake();err!=nil {
			m.record(Flow{Protocol:strings.ToUpper(proto)+" HTTPS MITM",Host:host,Error:"TLS handshake failed"})
			return
		}
		_ = conn.SetDeadline(time.Time{})
		srv:=&http.Server{Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){m.httpRequest(w,r,cfg,true,address)}),ReadHeaderTimeout:15*time.Second}
		_ = srv.Serve(&oneConnListener{conn:tlsConn})
		return
	}
	upstream,err:=net.DialTimeout("tcp",address,10*time.Second)
	if err!=nil {return}
	defer upstream.Close()
	if proto=="socks5" {
		_,_ = conn.Write([]byte{5,0,0,1,0,0,0,0,0,0})
	} else {
		_,_ = conn.Write([]byte{0,90,0,0,0,0,0,0})
	}
	_ = conn.SetDeadline(time.Time{})
	var client net.Conn=conn
	if reader.Buffered()>0 {client=&bufferedConn{Conn:conn,reader:reader}}
	pump(client,upstream)
	m.record(Flow{Protocol:strings.ToUpper(proto)+" tunnel",Host:host,DurationMS:time.Since(start).Milliseconds()})
}

// SOCKS5 deliberately supports CONNECT with NO AUTH on loopback only.
// UDP ASSOCIATE/BIND and credential authentication are not advertised.
func socks5Handshake(conn net.Conn,r *bufio.Reader)(string,string,error) {
	header:=make([]byte,2)
	if _,err:=io.ReadFull(r,header);err!=nil{return "","",err}
	if header[0]!=5 || header[1]==0 {return "","",errors.New("invalid SOCKS5 methods")}
	methods:=make([]byte,int(header[1]))
	if _,err:=io.ReadFull(r,methods);err!=nil{return "","",err}
	found:=false;for _,v:=range methods{if v==0{found=true}}
	if !found {_,_=conn.Write([]byte{5,255});return "","",errors.New("no supported SOCKS5 auth")}
	_,_=conn.Write([]byte{5,0})
	req:=make([]byte,4)
	if _,err:=io.ReadFull(r,req);err!=nil{return "","",err}
	if req[0]!=5 || req[1]!=1 || req[2]!=0 {return "","",errors.New("only SOCKS5 CONNECT supported")}
	var host string
	switch req[3] {
	case 1:
		ip:=make([]byte,4);if _,err:=io.ReadFull(r,ip);err!=nil{return "","",err};host=net.IP(ip).String()
	case 4:
		ip:=make([]byte,16);if _,err:=io.ReadFull(r,ip);err!=nil{return "","",err};host=net.IP(ip).String()
	case 3:
		length,err:=r.ReadByte();if err!=nil||length==0{return "","",errors.New("invalid SOCKS domain")}
		name:=make([]byte,int(length));if _,err:=io.ReadFull(r,name);err!=nil{return "","",err};host=string(name)
	default:return "","",errors.New("unsupported SOCKS5 address type")
	}
	p:=make([]byte,2);if _,err:=io.ReadFull(r,p);err!=nil{return "","",err}
	port:=int(p[0])<<8|int(p[1])
	if port==0{return "","",errors.New("invalid SOCKS5 port")}
	return net.JoinHostPort(host,strconv.Itoa(port)),host,nil
}

func readNullString(r *bufio.Reader,max int)(string,error){
	b:=make([]byte,0,32)
	for len(b)<max{c,err:=r.ReadByte();if err!=nil{return "",err};if c==0{return string(b),nil};b=append(b,c)}
	return "",errors.New("SOCKS string too long")
}

func socks4Handshake(conn net.Conn,r *bufio.Reader)(string,string,error){
	head:=make([]byte,8)
	if _,err:=io.ReadFull(r,head);err!=nil{return "","",err}
	if head[0]!=4 || head[1]!=1{return "","",errors.New("only SOCKS4 CONNECT supported")}
	port:=int(head[2])<<8|int(head[3]);if port==0{return "","",errors.New("invalid SOCKS4 port")}
	if _,err:=readNullString(r,255);err!=nil{return "","",err}
	host:=net.IP(head[4:8]).String()
	if head[4]==0 && head[5]==0 && head[6]==0 && head[7]!=0 {
		var err error
		host,err=readNullString(r,255);if err!=nil||host==""{return "","",errors.New("invalid SOCKS4a hostname")}
	}
	return net.JoinHostPort(host,strconv.Itoa(port)),host,nil
}

// Explicitly shut down listeners on backend lifecycle stop; nothing persists.
func (m *Manager) Close() {m.Stop()}

