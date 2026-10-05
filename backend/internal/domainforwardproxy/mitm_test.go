package domainforwardproxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTLSInterceptionStrictAllowlist(t *testing.T) {
	certFile, keyFile := writeTestCA(t)
	config, warnings, err := NewTLSConfig(DomainForwardProxySettings{
		TLSInterceptEnabled:        true,
		TLSInterceptAllowlist:      "api.openai.com, *.example.test, 127.0.0.1",
		TLSInterceptCACertFile:     certFile,
		TLSInterceptCAKeyFile:      keyFile,
		TLSInterceptLeafTTLSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("NewTLSConfig: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}

	for _, host := range []string{"api.openai.com", "worker.example.test", "127.0.0.1"} {
		certificate, err := config.GetCertificate(&tls.ClientHelloInfo{ServerName: host})
		if err != nil {
			t.Fatalf("GetCertificate(%s): %v", host, err)
		}
		leaf, err := x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			t.Fatalf("parse leaf: %v", err)
		}
		if err := leaf.VerifyHostname(host); err != nil {
			t.Fatalf("leaf does not cover %s: %v", host, err)
		}
	}
	for _, host := range []string{"example.test", "not-allowed.test"} {
		if _, err := config.GetCertificate(&tls.ClientHelloInfo{ServerName: host}); err == nil {
			t.Fatalf("non-allowlisted host %s unexpectedly received a certificate", host)
		}
	}
}

func TestTLSInterceptionRejectsInvalidCALifetime(t *testing.T) {
	cases := []struct {
		name      string
		notBefore time.Time
		notAfter  time.Time
	}{
		{
			name:      "not-yet-valid",
			notBefore: time.Now().Add(time.Hour),
			notAfter:  time.Now().Add(2 * time.Hour),
		},
		{
			name:      "expired",
			notBefore: time.Now().Add(-2 * time.Hour),
			notAfter:  time.Now().Add(-time.Hour),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			certFile, keyFile := writeTestCAWithValidity(t, tc.notBefore, tc.notAfter)
			_, _, err := NewTLSConfig(DomainForwardProxySettings{
				TLSInterceptEnabled:        true,
				TLSInterceptAllowlist:      "*.example.test",
				TLSInterceptCACertFile:     certFile,
				TLSInterceptCAKeyFile:      keyFile,
				TLSInterceptLeafTTLSeconds: 3600,
			})
			if err == nil {
				t.Fatal("expected invalid CA lifetime to be rejected")
			}
		})
	}
}

func TestTLSInterceptionLeafCacheIsBounded(t *testing.T) {
	certFile, keyFile := writeTestCA(t)
	ca, err := loadMITMCertificateAuthority(DomainForwardProxySettings{
		TLSInterceptEnabled:        true,
		TLSInterceptAllowlist:      "*.example.test",
		TLSInterceptCACertFile:     certFile,
		TLSInterceptCAKeyFile:      keyFile,
		TLSInterceptLeafTTLSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("loadMITMCertificateAuthority: %v", err)
	}

	now := time.Now()
	for i := 0; i < maxMITMLeafCacheEntries; i++ {
		host := fmt.Sprintf("cached-%04d.example.test", i)
		ca.cache[host] = cachedMITMCertificate{
			expiresAt: now.Add(time.Duration(i+1) * time.Second),
		}
	}
	if _, err := ca.certificateForHost("new.example.test"); err != nil {
		t.Fatalf("certificateForHost(new.example.test): %v", err)
	}
	if got := len(ca.cache); got != maxMITMLeafCacheEntries {
		t.Fatalf("cache size = %d, want %d", got, maxMITMLeafCacheEntries)
	}
	if _, ok := ca.cache["cached-0000.example.test"]; ok {
		t.Fatal("oldest cache entry was not evicted")
	}
	if _, ok := ca.cache["new.example.test"]; !ok {
		t.Fatal("newly issued certificate missing from bounded cache")
	}
}

func TestTLSInterceptionPreservesStaticFallback(t *testing.T) {
	certFile, keyFile := writeTestCA(t)
	config, _, err := NewTLSConfig(DomainForwardProxySettings{
		CertFile:                   certFile,
		KeyFile:                    keyFile,
		TLSInterceptEnabled:        true,
		TLSInterceptAllowlist:      "api.openai.com",
		TLSInterceptCACertFile:     certFile,
		TLSInterceptCAKeyFile:      keyFile,
		TLSInterceptLeafTTLSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("NewTLSConfig: %v", err)
	}

	certificate, err := config.GetCertificate(&tls.ClientHelloInfo{ServerName: "static-only.example"})
	if err != nil {
		t.Fatalf("static fallback rejected outside interception allowlist: %v", err)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		t.Fatalf("parse fallback certificate: %v", err)
	}
	if !leaf.IsCA {
		t.Fatal("outside-allowlist host unexpectedly received a dynamically issued MITM leaf")
	}
}

func writeTestCA(t *testing.T) (string, string) {
	t.Helper()
	now := time.Now()
	return writeTestCAWithValidity(t, now.Add(-time.Hour), now.Add(24*time.Hour))
}

func writeTestCAWithValidity(t *testing.T, notBefore, notAfter time.Time) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "agent-ebpf-filter test CA"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "ca.pem")
	keyFile := filepath.Join(dir, "ca-key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
