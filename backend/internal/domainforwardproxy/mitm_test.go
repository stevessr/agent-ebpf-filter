package domainforwardproxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
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

func writeTestCA(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "agent-ebpf-filter test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
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
