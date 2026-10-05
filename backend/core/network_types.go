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

// DomainBodyRewriteRule applies a bounded literal replacement to textual
// request/response bodies. Direction accepts request, response or both.
type DomainBodyRewriteRule struct {
	ID          string `json:"id,omitempty"`
	Enabled     bool   `json:"enabled"`
	Direction   string `json:"direction"`
	Host        string `json:"host,omitempty"`
	PathPrefix  string `json:"pathPrefix,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Find        string `json:"find"`
	Replace     string `json:"replace"`
}

// DomainModelRewriteRule redirects model identifiers on the proxy hot path.
// The response path restores the client-visible model for the same request or
// Responses WebSocket stream.
type DomainModelRewriteRule struct {
	Host string `json:"host,omitempty"`
	From string `json:"from"`
	To   string `json:"to"`
}

type DomainNativeInferenceSettings struct {
	Enabled       bool   `json:"enabled"`
	ModelFile     string `json:"modelFile,omitempty"`
	Direction     string `json:"direction,omitempty"`
	Host          string `json:"host,omitempty"`
	PathPrefix    string `json:"pathPrefix,omitempty"`
	ContentType   string `json:"contentType,omitempty"`
	MinTokenBytes int    `json:"minTokenBytes,omitempty"`
	MaxTokenBytes int    `json:"maxTokenBytes,omitempty"`
}

type DomainBodyRewriteSettings struct {
	Enabled      bool                          `json:"enabled"`
	MaxBodyBytes int64                         `json:"maxBodyBytes"`
	Rules        []DomainBodyRewriteRule       `json:"rules,omitempty"`
	ModelRules   []DomainModelRewriteRule      `json:"modelRules,omitempty"`
	Inference    DomainNativeInferenceSettings `json:"inference,omitempty"`
}

// DomainForwardProxySettings controls the optional public HTTP/HTTPS reverse proxy.
type DomainForwardProxySettings struct {
	Enabled                    bool                      `json:"enabled"`
	HTTPPort                   int                       `json:"httpPort"`
	HTTPSPort                  int                       `json:"httpsPort"`
	DefaultScheme              string                    `json:"defaultScheme"`
	AllowAnyHost               bool                      `json:"allowAnyHost"`
	DNSResolver                string                    `json:"dnsResolver,omitempty"`
	DialTimeoutSeconds         int                       `json:"dialTimeoutSeconds"`
	CertFile                   string                    `json:"certFile,omitempty"`
	KeyFile                    string                    `json:"keyFile,omitempty"`
	TLSInterceptEnabled        bool                      `json:"tlsInterceptEnabled"`
	TLSInterceptAllowlist      string                    `json:"tlsInterceptAllowlist,omitempty"`
	TLSInterceptCACertFile     string                    `json:"tlsInterceptCaCertFile,omitempty"`
	TLSInterceptCAKeyFile      string                    `json:"tlsInterceptCaKeyFile,omitempty"`
	TLSInterceptLeafTTLSeconds int                       `json:"tlsInterceptLeafTtlSeconds,omitempty"`
	Rewrite                    DomainBodyRewriteSettings `json:"rewrite,omitempty"`
	Routes                     []DomainForwardRoute      `json:"routes,omitempty"`
}
