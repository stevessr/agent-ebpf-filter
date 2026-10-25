package domainforwardproxy

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTLSInterceptionCompromisedCAModes(t *testing.T) {
	tests := []struct {
		name                  string
		mode                  string
		rootFile              string
		firstIssuerCommonName string
		secondCommonName      string
	}{
		{
			name:                  "emSign path first",
			mode:                  tlsInterceptCAModeEMudhra,
			rootFile:              "emSign_ecc_root_g3.pem",
			firstIssuerCommonName: "emSign Root TLS CA - G3",
			secondCommonName:      "emSign Root TLS CA - G3",
		},
		{
			name:                  "AAA cross-sign first for older OpenSSL",
			mode:                  tlsInterceptCAModeEMudhraOld,
			rootFile:              "aaa_certificate_services.pem",
			firstIssuerCommonName: "AAA Certificate Services",
			secondCommonName:      emudhraCompromisedCAName,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const host = "api.example.test"
			settings := DomainForwardProxySettings{
				TLSInterceptEnabled:        true,
				TLSInterceptCAMode:         test.mode,
				TLSInterceptAllowlist:      host,
				TLSInterceptCACertFile:     "/does-not-exist/ca.pem",
				TLSInterceptCAKeyFile:      "/does-not-exist/ca-key.pem",
				TLSInterceptLeafTTLSeconds: 3600,
			}
			config, warnings, err := NewTLSConfig(settings)
			if err != nil {
				if compromisedTestModeExpired(t, test.mode) && strings.Contains(err.Error(), "expired") {
					return
				}
				t.Fatalf("NewTLSConfig: %v", err)
			}
			if compromisedTestModeExpired(t, test.mode) {
				t.Fatal("NewTLSConfig accepted an expired CA verification path")
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], "publicly compromised") {
				t.Fatalf("warnings = %v, want visible compromised-key warning", warnings)
			}

			certificate, err := config.GetCertificate(&tls.ClientHelloInfo{ServerName: host})
			if err != nil {
				t.Fatalf("GetCertificate(%q): %v", host, err)
			}
			if got, want := len(certificate.Certificate), 4; got != want {
				t.Fatalf("certificate chain length = %d, want %d", got, want)
			}
			parsed := make([]*x509.Certificate, len(certificate.Certificate))
			for i, der := range certificate.Certificate {
				parsed[i], err = x509.ParseCertificate(der)
				if err != nil {
					t.Fatalf("parse chain certificate %d: %v", i, err)
				}
			}
			leaf := parsed[0]
			if err := leaf.VerifyHostname(host); err != nil {
				t.Fatalf("leaf does not verify host %q: %v", host, err)
			}
			if got := leaf.Issuer.CommonName; got != emudhraCompromisedCAName {
				t.Fatalf("leaf issuer = %q, want %q", got, emudhraCompromisedCAName)
			}
			if err := leaf.CheckSignatureFrom(parsed[1]); err != nil {
				t.Fatalf("leaf signature does not match first presented intermediate: %v", err)
			}
			if got := parsed[1].Issuer.CommonName; got != test.firstIssuerCommonName {
				t.Fatalf("first intermediate issuer = %q, want %q", got, test.firstIssuerCommonName)
			}
			if got := parsed[2].Subject.CommonName; got != test.secondCommonName {
				t.Fatalf("second intermediate subject = %q, want %q", got, test.secondCommonName)
			}

			if _, err := config.GetCertificate(&tls.ClientHelloInfo{ServerName: "not-allowed.test"}); err == nil {
				t.Fatal("non-allowlisted host received a dynamically signed certificate")
			}
			if statusMode := BuildStatus(DomainForwardProxySettings{TLSInterceptCAMode: test.mode}).TLSInterceptCAMode; statusMode != test.mode {
				t.Fatalf("status CA mode = %q, want %q", statusMode, test.mode)
			}
			if compromisedTestRootExpired(t, test.rootFile) {
				t.Skip("test trust root has expired; live path verification is no longer applicable")
			}

			// Verify against the actual roots used by each cross-sign path.
			roots := compromisedTestRootPool(t, test.rootFile)
			intermediates := x509.NewCertPool()
			for _, intermediate := range parsed[1:] {
				intermediates.AddCert(intermediate)
			}
			if _, err := leaf.Verify(x509.VerifyOptions{
				DNSName:       host,
				Roots:         roots,
				Intermediates: intermediates,
			}); err != nil {
				t.Fatalf("verify generated leaf through presented chain: %v", err)
			}
		})
	}
}

func TestTLSInterceptionCompromisedCAHTTPSHandshake(t *testing.T) {
	if compromisedTestModeExpired(t, tlsInterceptCAModeEMudhra) {
		t.Skip("emSign certificate path has expired")
	}
	if compromisedTestRootExpired(t, "emSign_ecc_root_g3.pem") {
		t.Skip("emSign trust root has expired")
	}
	const host = "api.example.test"
	serverConfig, warnings, err := NewTLSConfig(DomainForwardProxySettings{
		TLSInterceptEnabled:   true,
		TLSInterceptCAMode:    tlsInterceptCAModeEMudhra,
		TLSInterceptAllowlist: host,
	})
	if err != nil {
		t.Fatalf("NewTLSConfig: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want compromised-key warning", warnings)
	}
	roots := compromisedTestRootPool(t, "emSign_ecc_root_g3.pem")

	serverConn, clientConn := net.Pipe()
	server := tls.Server(serverConn, serverConfig)
	client := tls.Client(clientConn, &tls.Config{
		RootCAs:    roots,
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	})
	serverHandshake := make(chan error, 1)
	go func() { serverHandshake <- server.Handshake() }()
	if err := client.Handshake(); err != nil {
		_ = client.Close()
		_ = server.Close()
		t.Fatalf("client TLS handshake: %v", err)
	}
	if err := <-serverHandshake; err != nil {
		_ = client.Close()
		_ = server.Close()
		t.Fatalf("server TLS handshake: %v", err)
	}
	_ = client.Close()
	_ = server.Close()
}

func TestTLSInterceptCAModeDefaultValidationAndEquality(t *testing.T) {
	settings := DomainForwardProxySettings{}
	NormalizeSettings(&settings)
	if settings.TLSInterceptCAMode != tlsInterceptCAModeLocal {
		t.Fatalf("default CA mode = %q, want %q", settings.TLSInterceptCAMode, tlsInterceptCAModeLocal)
	}
	settings.TLSInterceptCAMode = " EMUDHRA-OLD "
	NormalizeSettings(&settings)
	if settings.TLSInterceptCAMode != tlsInterceptCAModeEMudhraOld {
		t.Fatalf("normalized CA mode = %q, want %q", settings.TLSInterceptCAMode, tlsInterceptCAModeEMudhraOld)
	}
	if SettingsEqual(
		DomainForwardProxySettings{TLSInterceptCAMode: tlsInterceptCAModeEMudhra},
		DomainForwardProxySettings{TLSInterceptCAMode: tlsInterceptCAModeEMudhraOld},
	) {
		t.Fatal("settings equality ignored a CA mode change")
	}
	if _, _, err := NewTLSConfig(DomainForwardProxySettings{
		TLSInterceptEnabled:   true,
		TLSInterceptCAMode:    "unknown",
		TLSInterceptAllowlist: "api.example.test",
	}); err == nil || !strings.Contains(err.Error(), "unsupported tlsInterceptCaMode") {
		t.Fatalf("unknown CA mode error = %v, want unsupported mode", err)
	}
}

func compromisedTestRootPool(t *testing.T, filename string) *x509.CertPool {
	t.Helper()
	pemBytes, err := os.ReadFile("testdata/" + filename)
	if err != nil {
		t.Fatalf("read test root %q: %v", filename, err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		t.Fatalf("load test root %q", filename)
	}
	return roots
}

func TestTLSInterceptionClampsLeafExpiryToVerificationPath(t *testing.T) {
	if compromisedTestModeExpired(t, tlsInterceptCAModeEMudhra) {
		t.Skip("emSign certificate path has expired")
	}
	ca, err := loadMITMCertificateAuthority(DomainForwardProxySettings{
		TLSInterceptEnabled:   true,
		TLSInterceptCAMode:    tlsInterceptCAModeEMudhra,
		TLSInterceptAllowlist: "api.example.test",
	})
	if err != nil {
		t.Fatalf("loadMITMCertificateAuthority: %v", err)
	}
	pathCertificate, err := x509.ParseCertificate(ca.chain[1])
	if err != nil {
		t.Fatalf("parse verification-path certificate: %v", err)
	}
	if !ca.validUntil.Equal(pathCertificate.NotAfter) {
		t.Fatalf("effective path expiry = %s, want intermediate expiry %s",
			ca.validUntil.UTC().Format(time.RFC3339),
			pathCertificate.NotAfter.UTC().Format(time.RFC3339))
	}
	validUntil := time.Now().Add(time.Minute)
	ca.validUntil = validUntil

	certificate, err := ca.certificateForHost("api.example.test")
	if err != nil {
		t.Fatalf("certificateForHost: %v", err)
	}
	if certificate.Leaf.NotAfter.After(validUntil) {
		t.Fatalf("leaf expires at %s, after verification path at %s",
			certificate.Leaf.NotAfter.UTC().Format(time.RFC3339),
			validUntil.UTC().Format(time.RFC3339))
	}
}

func compromisedTestModeExpired(t *testing.T, mode string) bool {
	t.Helper()
	cert, chain, _, err := loadCompromisedEMudhraCAMaterial(mode)
	if err != nil {
		t.Fatalf("load test CA material for %q: %v", mode, err)
	}
	validUntil := cert.NotAfter
	if mode == tlsInterceptCAModeEMudhra {
		if len(chain) < 2 {
			t.Fatal("emSign test path is incomplete")
		}
		pathCertificate, err := x509.ParseCertificate(chain[1])
		if err != nil {
			t.Fatalf("parse emSign path certificate: %v", err)
		}
		if pathCertificate.NotAfter.Before(validUntil) {
			validUntil = pathCertificate.NotAfter
		}
	}
	return !time.Now().Before(validUntil)
}

func compromisedTestRootExpired(t *testing.T, filename string) bool {
	t.Helper()
	pemBytes, err := os.ReadFile("testdata/" + filename)
	if err != nil {
		t.Fatalf("read test root %q: %v", filename, err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		t.Fatalf("decode test root %q", filename)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse test root %q: %v", filename, err)
	}
	return !time.Now().Before(cert.NotAfter)
}
