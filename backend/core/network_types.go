package core

// ── Domain forward proxy types ───────────────────────────────────────────────

// DomainForwardRoute maps one request host/SNI name to an optional upstream.
// When Upstream is empty, the proxy forwards to <defaultScheme>://<request-host>.
type DomainForwardRoute struct {
	Host     string `json:"host"`
	Upstream string `json:"upstream,omitempty"`
	CertFile string `json:"certFile,omitempty"`
	KeyFile  string `json:"keyFile,omitempty"`
}

// DomainForwardProxySettings controls the optional public HTTP/HTTPS reverse proxy.
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
