package domainforwardproxy

import (
	"bytes"
	"crypto"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

const (
	tlsInterceptCAModeLocal      = "local"
	tlsInterceptCAModeEMudhra    = "emudhra"
	tlsInterceptCAModeEMudhraOld = "emudhra-old"
	emudhraCompromisedCAName     = "EM OV TLS CA - G3C"
	emudhraCompromiseDisclosure  = "https://emudhra.we-have-a-problem.com/"
)

// This key is intentionally embedded: the eMudhra EM OV TLS CA - G3C
// intermediate private key was publicly disclosed. Use only for authorized
// debugging. It remains gated by TLS interception and the host allowlist.
// The bundle and chain ordering follow mitmproxy PR #8503.
//
//go:embed assets/emudhra_compromised_chain.pem
var emudhraCompromisedChainPEM []byte

func normalizeTLSInterceptCAMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return tlsInterceptCAModeLocal
	}
	return mode
}

func isCompromisedTLSInterceptCAMode(mode string) bool {
	switch normalizeTLSInterceptCAMode(mode) {
	case tlsInterceptCAModeEMudhra, tlsInterceptCAModeEMudhraOld:
		return true
	default:
		return false
	}
}

func compromisedTLSInterceptWarning(mode string) string {
	order := "emSign-issued intermediate first"
	if normalizeTLSInterceptCAMode(mode) == tlsInterceptCAModeEMudhraOld {
		order = "AAA Certificate Services cross-sign first"
	}
	return fmt.Sprintf(
		"TLS interception CA mode %q uses the publicly compromised %s private key (%s; %s); use only for authorized debugging, and note that trust stores may reject or distrust this CA",
		normalizeTLSInterceptCAMode(mode), emudhraCompromisedCAName, emudhraCompromiseDisclosure, order,
	)
}

func loadCompromisedEMudhraCAMaterial(mode string) (*x509.Certificate, [][]byte, crypto.Signer, error) {
	mode = normalizeTLSInterceptCAMode(mode)
	if !isCompromisedTLSInterceptCAMode(mode) {
		return nil, nil, nil, fmt.Errorf("unsupported compromised TLS interception CA mode %q", mode)
	}

	keyBlock, rest := pem.Decode(emudhraCompromisedChainPEM)
	if keyBlock == nil || keyBlock.Type != "PRIVATE KEY" {
		return nil, nil, nil, errors.New("embedded eMudhra TLS interception private key is invalid")
	}
	signer, err := parsePrivateSigner(pem.EncodeToMemory(keyBlock))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse embedded eMudhra TLS interception key: %w", err)
	}

	certificates := make([]*x509.Certificate, 0, 3)
	for len(bytes.TrimSpace(rest)) > 0 {
		block, next := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, nil, nil, errors.New("embedded eMudhra TLS interception certificate chain is invalid")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parse embedded eMudhra TLS interception certificate: %w", err)
		}
		certificates = append(certificates, cert)
		rest = next
	}
	if len(certificates) != 3 {
		return nil, nil, nil, fmt.Errorf("embedded eMudhra TLS interception chain has %d certificates, want 3", len(certificates))
	}

	// The first and last certificates are cross-signs of the same compromised
	// intermediate. The middle certificate links the emSign path to its root.
	if !bytes.Equal(certificates[0].RawSubject, certificates[2].RawSubject) ||
		!bytes.Equal(certificates[0].SubjectKeyId, certificates[2].SubjectKeyId) ||
		!publicKeysEqual(certificates[0].PublicKey, certificates[2].PublicKey) {
		return nil, nil, nil, errors.New("embedded eMudhra cross-signed intermediates do not identify the same CA")
	}
	if err := certificates[0].CheckSignatureFrom(certificates[1]); err != nil {
		return nil, nil, nil, fmt.Errorf("verify embedded eMudhra emSign chain: %w", err)
	}

	ordered := certificates
	if mode == tlsInterceptCAModeEMudhraOld {
		ordered = []*x509.Certificate{certificates[2], certificates[0], certificates[1]}
	}
	chain := make([][]byte, 0, len(ordered))
	for _, cert := range ordered {
		chain = append(chain, cert.Raw)
	}
	return ordered[0], chain, signer, nil
}
