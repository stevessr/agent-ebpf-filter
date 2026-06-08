package rules

import (
	"agent-ebpf-filter/redaction"
	"fmt"
	"net"
	"strings"

	"agent-ebpf-filter/internal/network"
)

const (
	redactedIPPlaceholder     = "[REDACTED_IP]"
	redactedDomainPlaceholder = "[REDACTED_DOMAIN]"
)

// RedactIP applies redaction rules to an IP address string.
func RedactIP(ip string, level redaction.RedactionLevel) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ip
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}

	switch level {
	case redaction.RedactionLevelNone, redaction.RedactionLevelBasic:
		return ip
	case redaction.RedactionLevelStandard:
		if isPrivateIP(ip) {
			return ipScopePlaceholder(parsed)
		}
		return ip
	case redaction.RedactionLevelStrict:
		return redactedIPPlaceholder
	default:
		return ip
	}
}

// RedactDomain applies redaction rules to a domain name string.
func RedactDomain(domain string, level redaction.RedactionLevel) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return domain
	}

	switch level {
	case redaction.RedactionLevelNone, redaction.RedactionLevelBasic:
		return domain
	case redaction.RedactionLevelStandard:
		if isInternalDomain(domain) {
			return genericDomainPlaceholder(domain)
		}
		return domain
	case redaction.RedactionLevelStrict:
		return redactedDomainPlaceholder
	default:
		return domain
	}
}

// isPrivateIP reports whether the IP belongs to a local, private, or otherwise internal scope.
func isPrivateIP(ip string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	scope := network.ClassifyIPScope(parsed)
	switch scope {
	case network.ScopeLoopback, network.ScopePrivate, network.ScopeLinkLocal, network.ScopeCGNAT, network.ScopeUniqueLocal:
		return true
	default:
		return false
	}
}

// isInternalDomain reports whether the domain looks like an internal or local DNS name.
func isInternalDomain(domain string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return false
	}
	if domain == "localhost" || domain == "localhost.localdomain" {
		return true
	}
	if strings.HasSuffix(domain, ".local") || strings.HasSuffix(domain, ".internal") || strings.HasSuffix(domain, ".intranet") {
		return true
	}
	if strings.Count(domain, ".") == 0 {
		return true
	}
	return false
}

func ipScopePlaceholder(ip net.IP) string {
	switch network.ClassifyIPScope(ip) {
	case network.ScopeLoopback:
		return "[LOOPBACK_IP]"
	case network.ScopePrivate:
		return "[PRIVATE_IP]"
	case network.ScopeLinkLocal:
		return "[LINK_LOCAL_IP]"
	case network.ScopeCGNAT:
		return "[CGNAT_IP]"
	case network.ScopeUniqueLocal:
		return "[ULA_IP]"
	default:
		return redactedIPPlaceholder
	}
}

func genericDomainPlaceholder(domain string) string {
	if strings.HasSuffix(domain, ".local") {
		return "[LOCAL_DOMAIN]"
	}
	if strings.HasSuffix(domain, ".internal") {
		return "[INTERNAL_DOMAIN]"
	}
	if strings.HasSuffix(domain, ".intranet") {
		return "[INTRANET_DOMAIN]"
	}
	if strings.Count(domain, ".") == 0 {
		return "[LOCAL_DOMAIN]"
	}
	return fmt.Sprintf("[DOMAIN:%d]", len(domain))
}
