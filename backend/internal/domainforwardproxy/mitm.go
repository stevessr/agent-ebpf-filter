package domainforwardproxy

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"
)

type mitmCertificateAuthority struct {
	cert       *x509.Certificate
	signer     crypto.Signer
	leafTTL    time.Duration
	allowlist  []string
	mu         sync.Mutex
	cache      map[string]cachedMITMCertificate
}

type cachedMITMCertificate struct {
	cert      *tls.Certificate
	expiresAt time.Time
}

func loadMITMCertificateAuthority(settings DomainForwardProxySettings) (*mitmCertificateAuthority, error) {
	if !settings.TLSInterceptEnabled {
		return nil, nil
	}
	if strings.TrimSpace(settings.TLSInterceptCACertFile) == "" || strings.TrimSpace(settings.TLSInterceptCAKeyFile) == "" {
		return nil, errors.New("TLS interception requires tlsInterceptCaCertFile and tlsInterceptCaKeyFile")
	}
	allowlist := splitDomainAllowlist(settings.TLSInterceptAllowlist)
	if len(allowlist) == 0 {
		return nil, errors.New("TLS interception requires a non-empty tlsInterceptAllowlist")
	}
	certPEM, err := os.ReadFile(settings.TLSInterceptCACertFile)
	if err != nil {
		return nil, fmt.Errorf("read TLS interception CA certificate: %w", err)
	}
	keyPEM, err := os.ReadFile(settings.TLSInterceptCAKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read TLS interception CA key: %w", err)
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, errors.New("TLS interception CA certificate is not PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse TLS interception CA certificate: %w", err)
	}
	if !cert.IsCA {
		return nil, errors.New("TLS interception certificate is not a CA")
	}
	signer, err := parsePrivateSigner(keyPEM)
	if err != nil {
		return nil, err
	}
	if cert.PublicKey == nil || signer.Public() == nil {
		return nil, errors.New("TLS interception CA key is invalid")
	}
	if !publicKeysEqual(cert.PublicKey, signer.Public()) {
		return nil, errors.New("TLS interception CA certificate and private key do not match")
	}
	ttl := time.Duration(settings.TLSInterceptLeafTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	if ttl > 7*24*time.Hour {
		ttl = 7 * 24 * time.Hour
	}
	return &mitmCertificateAuthority{
		cert:      cert,
		signer:    signer,
		leafTTL:   ttl,
		allowlist: allowlist,
		cache:     make(map[string]cachedMITMCertificate),
	}, nil
}

func (ca *mitmCertificateAuthority) allowed(host string) bool {
	host = NormalizeForwardHost(host)
	if ca == nil || host == "" {
		return false
	}
	for _, pattern := range ca.allowlist {
		if rewriteHostMatches(pattern, host) {
			return true
		}
	}
	return false
}

func (ca *mitmCertificateAuthority) certificateForHost(host string) (*tls.Certificate, error) {
	host = NormalizeForwardHost(host)
	if !ca.allowed(host) {
		return nil, fmt.Errorf("TLS interception is not allowlisted for %q", host)
	}
	now := time.Now()
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if cached, ok := ca.cache[host]; ok && now.Before(cached.expiresAt.Add(-time.Minute)) {
		return cached.cert, nil
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate TLS interception leaf key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, fmt.Errorf("generate TLS interception serial: %w", err)
	}
	notAfter := now.Add(ca.leafTTL)
	if ca.cert.NotAfter.Before(notAfter) {
		notAfter = ca.cert.NotAfter
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &leafKey.PublicKey, ca.signer)
	if err != nil {
		return nil, fmt.Errorf("sign TLS interception certificate: %w", err)
	}
	cert := &tls.Certificate{
		Certificate: [][]byte{der, ca.cert.Raw},
		PrivateKey:  leafKey,
		Leaf:        template,
	}
	expiresAt := notAfter
	ca.cache[host] = cachedMITMCertificate{cert: cert, expiresAt: expiresAt}
	return cert, nil
}

func splitDomainAllowlist(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		pattern := NormalizeDomainPattern(field)
		if pattern == "" {
			continue
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		out = append(out, pattern)
	}
	return out
}

func parsePrivateSigner(keyPEM []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("TLS interception CA key is not PEM")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if signer, ok := key.(crypto.Signer); ok {
			return signer, nil
		}
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("unsupported TLS interception CA private key")
}

func publicKeysEqual(a, b any) bool {
	left, err := x509.MarshalPKIXPublicKey(a)
	if err != nil {
		return false
	}
	right, err := x509.MarshalPKIXPublicKey(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}
