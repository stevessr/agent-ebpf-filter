package network

import (
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
)

type NetworkAuditFinding struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type NetworkAuditFlags struct {
	SuspiciousPort bool `json:"suspiciousPort"`
	ReverseShell   bool `json:"reverseShell"`
	DataExfil      bool `json:"dataExfil"`
	DNSTunnel      bool `json:"dnsTunnel"`
	ClearTextProto bool `json:"clearTextProto"`
	UnusualTarget  bool `json:"unusualTarget"`
	PortScan       bool `json:"portScan"`
	FirewallManip  bool `json:"firewallManip"`
}

type NetworkAuditResult struct {
	RiskScore float64               `json:"riskScore"`
	RiskLevel string                `json:"riskLevel"`
	Findings  []NetworkAuditFinding `json:"findings"`
	Flags     NetworkAuditFlags     `json:"flags"`
}

var (
	auditNetcatBins = map[string]struct{}{
		"nc": {}, "ncat": {}, "netcat": {}, "nc.openbsd": {}, "nc.traditional": {},
	}
	auditClearTextBins = map[string]struct{}{
		"ftp": {}, "telnet": {}, "tftp": {},
	}
	auditSuspiciousPorts = map[string]struct{}{
		"4444": {}, "1337": {}, "31337": {},
		"6666": {}, "6667": {}, "6668": {}, "6669": {},
		"8888": {}, "9999": {},
	}
	loopbackExemptDevPorts = map[string]struct{}{
		"8888": {}, "9999": {},
	}
)

func severityRank(s string) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func severityScore(s string) float64 {
	switch s {
	case "critical":
		return 25.0
	case "high":
		return 15.0
	case "medium":
		return 10.0
	case "low":
		return 5.0
	default:
		return 0
	}
}

func AuditNetworkBehavior(comm, cmdline string) NetworkAuditResult {
	result := NetworkAuditResult{Findings: []NetworkAuditFinding{}}
	commBase := strings.ToLower(filepath.Base(strings.TrimSpace(comm)))
	if commBase == "." || commBase == "/" {
		commBase = ""
	}
	lowerCmd := strings.ToLower(strings.TrimSpace(cmdline))
	tokens := strings.Fields(lowerCmd)
	fullCmd := strings.TrimSpace(commBase + " " + lowerCmd)

	_, isNetcat := auditNetcatBins[commBase]
	_, isClearText := auditClearTextBins[commBase]

	// Reverse shell patterns
	if ((isNetcat || hasCommandBinaryToken(tokens, auditNetcatBins)) && slices.Contains(tokens, "-e")) ||
		strings.Contains(fullCmd, "bash -i >& /dev/tcp") || strings.Contains(fullCmd, "bash -i > /dev/tcp") ||
		strings.Contains(fullCmd, "python -c 'import socket") || strings.Contains(fullCmd, `python -c "import socket`) ||
		strings.Contains(fullCmd, "socat exec:") || strings.Contains(fullCmd, "perl -e 'use socket") {
		result.Flags.ReverseShell = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "reverse_shell",
			Severity:    "critical",
			Description: "Reverse shell pattern detected",
		})
	}

	// Data exfiltration patterns
	if (strings.Contains(fullCmd, "curl -d @") || strings.Contains(fullCmd, "curl --data @") ||
		strings.Contains(fullCmd, "wget --post-file") || ((isNetcat || hasCommandBinaryToken(tokens, auditNetcatBins)) && strings.Contains(lowerCmd, "<"))) &&
		(strings.Contains(lowerCmd, "/etc/passwd") || strings.Contains(lowerCmd, "/etc/shadow") ||
			strings.Contains(lowerCmd, "~/.ssh") || strings.Contains(lowerCmd, "/.ssh/") || strings.Contains(lowerCmd, "~/.aws")) {
		result.Flags.DataExfil = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "data_exfil",
			Severity:    "critical",
			Description: "Data exfiltration pattern detected",
		})
	}

	// DNS tunnel patterns (exact executable basename avoids false hits like "digest")
	if commBase == "iodine" || commBase == "dnscat" || commBase == "dnscat2" ||
		(commBase == "dig" && slices.Contains(tokens, "+tcp")) {
		result.Flags.DNSTunnel = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "dns_tunnel",
			Severity:    "high",
			Description: "DNS tunneling tool detected",
		})
	}

	// Port scanning patterns
	if (commBase == "nmap" && (slices.Contains(tokens, "-ss") || slices.Contains(tokens, "-st"))) ||
		(isNetcat && hasPortScanFlags(tokens)) ||
		commBase == "masscan" {
		result.Flags.PortScan = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "port_scan",
			Severity:    "high",
			Description: "Port scanning activity detected",
		})
	}

	// Suspicious ports with exact boundary checks (avoids :44440 false hits and loopback dev servers)
	if port, ok := findSuspiciousAuditPort(tokens); ok {
		result.Flags.SuspiciousPort = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "suspicious_port",
			Severity:    "high",
			Description: "Connection to suspicious port " + port,
		})
	}

	// Cleartext protocols (exact binary match excludes encrypted sftp / sftp-server)
	if isClearText && !slices.Contains(tokens, "-i") {
		result.Flags.ClearTextProto = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "cleartext",
			Severity:    "medium",
			Description: "Cleartext protocol usage detected",
		})
	}

	// Firewall manipulation
	if (commBase == "iptables" && (slices.Contains(tokens, "-f") || slices.Contains(tokens, "-x"))) ||
		(commBase == "ufw" && slices.Contains(tokens, "disable")) ||
		(commBase == "firewall-cmd" && hasTokenPrefix(tokens, "--remove-service")) {
		result.Flags.FirewallManip = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "firewall_manipulation",
			Severity:    "critical",
			Description: "Firewall manipulation detected",
		})
	}

	score := 0.0
	maxSeverity := "safe"
	for _, f := range result.Findings {
		score += severityScore(f.Severity)
		if severityRank(f.Severity) > severityRank(maxSeverity) {
			maxSeverity = f.Severity
		}
	}
	result.RiskScore = min(score, 100.0)
	result.RiskLevel = strings.ToUpper(maxSeverity)
	return result
}

func hasCommandBinaryToken(tokens []string, bins map[string]struct{}) bool {
	for _, tok := range tokens {
		base := filepath.Base(tok)
		if _, ok := bins[base]; ok {
			return true
		}
	}
	return false
}

func hasPortScanFlags(tokens []string) bool {
	return slices.Contains(tokens, "-zv") || (slices.Contains(tokens, "-z") && slices.Contains(tokens, "-v"))
}

func hasTokenPrefix(tokens []string, prefix string) bool {
	for _, tok := range tokens {
		if strings.HasPrefix(tok, prefix) {
			return true
		}
	}
	return false
}

func findSuspiciousAuditPort(tokens []string) (string, bool) {
	for i, raw := range tokens {
		tok := strings.Trim(raw, `"'(),;`)
		if tok == "" {
			continue
		}
		if host, port, ok := extractHostAndPort(tok); ok {
			if _, hit := auditSuspiciousPorts[port]; !hit {
				continue
			}
			if _, devExempt := loopbackExemptDevPorts[port]; devExempt && isLoopbackAuditHost(host) {
				continue
			}
			return port, true
		}
		if i > 0 && (tokens[i-1] == "-p" || tokens[i-1] == "--port") {
			if _, hit := auditSuspiciousPorts[tok]; hit {
				return tok, true
			}
		}
	}
	return "", false
}

func extractHostAndPort(tok string) (host, port string, ok bool) {
	if strings.Contains(tok, "://") {
		if u, err := url.Parse(tok); err == nil && u.Host != "" {
			tok = u.Host
		}
	}
	if h, p, err := net.SplitHostPort(tok); err == nil && p != "" {
		return strings.ToLower(h), p, true
	}
	return "", "", false
}

func isLoopbackAuditHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	return addr.IsLoopback()
}
