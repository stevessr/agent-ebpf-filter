package domainforwardproxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type DomainForwardRoute struct {
	Host     string `json:"host"`
	Upstream string `json:"upstream,omitempty"`
	CertFile string `json:"certFile,omitempty"`
	KeyFile  string `json:"keyFile,omitempty"`
}

type DomainForwardProxySettings struct {
	Enabled            bool                 `json:"enabled"`
	HTTPPort           int                  `json:"httpPort"`
	HTTPSPort          int                  `json:"httpsPort"`
	DefaultScheme      string               `json:"defaultScheme"`
	AllowAnyHost       bool                 `json:"allowAnyHost"`
	DNSResolver        string               `json:"dnsResolver,omitempty"`
	DialTimeoutSeconds int                  `json:"dialTimeoutSeconds"`
	CertFile           string               `json:"certFile,omitempty"`
	KeyFile            string               `json:"keyFile,omitempty"`
	Routes             []DomainForwardRoute `json:"routes,omitempty"`
}

type Status struct {
	Enabled      bool      `json:"enabled"`
	HTTPRunning  bool      `json:"httpRunning"`
	HTTPSRunning bool      `json:"httpsRunning"`
	HTTPAddress  string    `json:"httpAddress,omitempty"`
	HTTPSAddress string    `json:"httpsAddress,omitempty"`
	HTTPPort     int       `json:"httpPort"`
	HTTPSPort    int       `json:"httpsPort"`
	RouteCount   int       `json:"routeCount"`
	AllowAnyHost bool      `json:"allowAnyHost"`
	DNSResolver  string    `json:"dnsResolver,omitempty"`
	Errors       []string  `json:"errors,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Runtime struct {
	mu          sync.Mutex
	active      bool
	httpServer  *http.Server
	httpsServer *http.Server
	settings    DomainForwardProxySettings
	status      Status
}

func NewRuntime() *Runtime {
	settings := DomainForwardProxySettings{}
	NormalizeSettings(&settings)
	return &Runtime{
		settings: settings,
		status: Status{
			Enabled:    false,
			HTTPPort:   settings.HTTPPort,
			HTTPSPort:  settings.HTTPSPort,
			UpdatedAt:  time.Now().UTC(),
			RouteCount: 0,
		},
	}
}

func (m *Runtime) Activate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = true
}

func (m *Runtime) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeLocked()
	m.active = false
	m.status.HTTPRunning = false
	m.status.HTTPSRunning = false
	m.status.HTTPAddress = ""
	m.status.HTTPSAddress = ""
	m.status.UpdatedAt = time.Now().UTC()
}

func (m *Runtime) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := m.status
	if len(m.status.Errors) > 0 {
		status.Errors = append([]string(nil), m.status.Errors...)
	}
	return status
}

func (m *Runtime) Apply(settings DomainForwardProxySettings) error {
	NormalizeSettings(&settings)

	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active {
		m.settings = settings
		m.status = BuildStatus(settings)
		return nil
	}
	if SettingsEqual(m.settings, settings) {
		return nil
	}

	m.closeLocked()
	m.settings = settings
	m.status = BuildStatus(settings)
	if !settings.Enabled {
		return nil
	}

	handler := NewHandler(settings)
	if settings.HTTPPort > 0 {
		if err := m.startHTTPServerLocked(settings, handler); err != nil {
			m.status.Errors = append(m.status.Errors, fmt.Sprintf("http listener: %v", err))
		}
	}
	if settings.HTTPSPort > 0 {
		if err := m.startHTTPSServerLocked(settings, handler); err != nil {
			m.status.Errors = append(m.status.Errors, fmt.Sprintf("https listener: %v", err))
		}
	}
	m.status.UpdatedAt = time.Now().UTC()
	if !m.status.HTTPRunning && !m.status.HTTPSRunning {
		return errors.New(strings.Join(m.status.Errors, "; "))
	}
	return nil
}

func (m *Runtime) startHTTPServerLocked(settings DomainForwardProxySettings, handler http.Handler) error {
	addr := fmt.Sprintf(":%d", settings.HTTPPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	m.httpServer = server
	m.status.HTTPRunning = true
	m.status.HTTPAddress = listener.Addr().String()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[DOMAIN-FORWARD] HTTP proxy stopped: %v", err)
			m.setRuntimeError(fmt.Sprintf("http listener stopped: %v", err))
		}
	}()
	log.Printf("[DOMAIN-FORWARD] HTTP proxy listening on %s", listener.Addr())
	return nil
}

func (m *Runtime) startHTTPSServerLocked(settings DomainForwardProxySettings, handler http.Handler) error {
	tlsConfig, warnings, err := NewTLSConfig(settings)
	m.status.Errors = append(m.status.Errors, warnings...)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf(":%d", settings.HTTPSPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	m.httpsServer = server
	m.status.HTTPSRunning = true
	m.status.HTTPSAddress = listener.Addr().String()
	tlsListener := tls.NewListener(listener, tlsConfig)
	go func() {
		if err := server.Serve(tlsListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[DOMAIN-FORWARD] HTTPS proxy stopped: %v", err)
			m.setRuntimeError(fmt.Sprintf("https listener stopped: %v", err))
		}
	}()
	log.Printf("[DOMAIN-FORWARD] HTTPS proxy listening on %s", listener.Addr())
	return nil
}

func (m *Runtime) closeLocked() {
	shutdown := func(server *http.Server) {
		if server == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[DOMAIN-FORWARD] failed to stop proxy listener: %v", err)
		}
	}
	shutdown(m.httpServer)
	shutdown(m.httpsServer)
	m.httpServer = nil
	m.httpsServer = nil
}

func (m *Runtime) setRuntimeError(message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.Errors = append(m.status.Errors, message)
	m.status.UpdatedAt = time.Now().UTC()
}

func BuildStatus(settings DomainForwardProxySettings) Status {
	return Status{
		Enabled:      settings.Enabled,
		HTTPPort:     settings.HTTPPort,
		HTTPSPort:    settings.HTTPSPort,
		RouteCount:   len(settings.Routes),
		AllowAnyHost: settings.AllowAnyHost,
		DNSResolver:  settings.DNSResolver,
		UpdatedAt:    time.Now().UTC(),
	}
}
