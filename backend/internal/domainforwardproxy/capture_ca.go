package domainforwardproxy

import (
	"crypto/tls"
	"fmt"
	"strings"
)

// NewCaptureTLSConfig reuses the reverse proxy's constrained CA signer for
// explicit, allowlisted forward-proxy interception. A CONNECT destination must
// match the selected certificate; clients cannot ask us to sign an arbitrary SNI.
func NewCaptureTLSConfig(certFile, keyFile, allowlist, connectHost string) (*tls.Config, error) {
	ca, err := loadMITMCertificateAuthority(DomainForwardProxySettings{
		TLSInterceptEnabled: true,
		TLSInterceptCACertFile: certFile,
		TLSInterceptCAKeyFile: keyFile,
		TLSInterceptAllowlist: allowlist,
	})
	if err != nil {
		return nil, err
	}
	host := NormalizeForwardHost(connectHost)
	if !ca.allowed(host) {
		return nil, fmt.Errorf("CONNECT host %q is not in the TLS interception allowlist", host)
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if hello != nil && hello.ServerName != "" && !strings.EqualFold(NormalizeForwardHost(hello.ServerName), host) {
				return nil, fmt.Errorf("TLS SNI does not match CONNECT host %q", host)
			}
			return ca.certificateForHost(host)
		},
	}, nil
}
