package main

import (
	"strings"
)

type NetworkAuditFinding struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type NetworkAuditFlags struct {
	SuspiciousPort  bool `json:"suspiciousPort"`
	ReverseShell    bool `json:"reverseShell"`
	DataExfil       bool `json:"dataExfil"`
	DNSTunnel       bool `json:"dnsTunnel"`
	ClearTextProto  bool `json:"clearTextProto"`
	UnusualTarget   bool `json:"unusualTarget"`
	PortScan        bool `json:"portScan"`
	FirewallManip   bool `json:"firewallManip"`
}

type NetworkAuditResult struct {
	RiskScore float64               `json:"riskScore"`
	RiskLevel string                `json:"riskLevel"`
	Findings  []NetworkAuditFinding `json:"findings"`
	Flags     NetworkAuditFlags     `json:"flags"`
}

func AuditNetworkBehavior(comm, cmdline string) NetworkAuditResult {
	result := NetworkAuditResult{Findings: []NetworkAuditFinding{}}
	lower := strings.ToLower(cmdline)
	fullCmd := strings.ToLower(comm + " " + cmdline)

	// Reverse shell patterns
	if strings.Contains(fullCmd, "nc -e") || strings.Contains(fullCmd, "nc.traditional -e") ||
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
		strings.Contains(fullCmd, "wget --post-file") || strings.Contains(fullCmd, "nc <")) &&
		(strings.Contains(lower, "/etc/passwd") || strings.Contains(lower, "/etc/shadow") ||
			strings.Contains(lower, "~/.ssh") || strings.Contains(lower, "~/.aws")) {
		result.Flags.DataExfil = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "data_exfil",
			Severity:    "critical",
			Description: "Data exfiltration pattern detected",
		})
	}

	// DNS tunnel patterns
	if strings.Contains(comm, "iodine") || strings.Contains(comm, "dnscat") ||
		(strings.Contains(comm, "dig") && strings.Contains(lower, "+tcp")) {
		result.Flags.DNSTunnel = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "dns_tunnel",
			Severity:    "high",
			Description: "DNS tunneling tool detected",
		})
	}

	// Port scanning patterns
	if (strings.Contains(comm, "nmap") && (strings.Contains(lower, "-ss") || strings.Contains(lower, "-st"))) ||
		(strings.Contains(comm, "nc") && strings.Contains(lower, "-zv")) ||
		strings.Contains(comm, "masscan") {
		result.Flags.PortScan = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "port_scan",
			Severity:    "high",
			Description: "Port scanning activity detected",
		})
	}

	// Suspicious ports (C2 common ports)
	suspiciousPorts := []string{"4444", "1337", "31337", "6666", "6667", "6668", "6669", "8888", "9999"}
	for _, port := range suspiciousPorts {
		if strings.Contains(cmdline, ":"+port) || strings.Contains(cmdline, " "+port+" ") {
			result.Flags.SuspiciousPort = true
			result.Findings = append(result.Findings, NetworkAuditFinding{
				Type:        "suspicious_port",
				Severity:    "high",
				Description: "Connection to suspicious port " + port,
			})
			break
		}
	}

	// Cleartext protocols
	if (strings.Contains(comm, "ftp") || strings.Contains(comm, "telnet")) &&
		!strings.Contains(lower, "-i") {
		result.Flags.ClearTextProto = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "cleartext",
			Severity:    "medium",
			Description: "Cleartext protocol usage detected",
		})
	}

	// Firewall manipulation
	if (strings.Contains(comm, "iptables") && (strings.Contains(lower, "-f") || strings.Contains(lower, "-x"))) ||
		(strings.Contains(comm, "ufw") && strings.Contains(lower, "disable")) ||
		(strings.Contains(comm, "firewall-cmd") && strings.Contains(lower, "--remove-service")) {
		result.Flags.FirewallManip = true
		result.Findings = append(result.Findings, NetworkAuditFinding{
			Type:        "firewall_manipulation",
			Severity:    "critical",
			Description: "Firewall manipulation detected",
		})
	}

	// Compute risk score and level
	score := 0.0
	maxSeverity := "safe"
	for _, f := range result.Findings {
		switch f.Severity {
		case "critical":
			score += 25.0
			maxSeverity = "critical"
		case "high":
			score += 15.0
			if maxSeverity != "critical" {
				maxSeverity = "high"
			}
		case "medium":
			score += 10.0
			if maxSeverity != "critical" && maxSeverity != "high" {
				maxSeverity = "medium"
			}
		case "low":
			score += 5.0
			if maxSeverity == "safe" {
				maxSeverity = "low"
			}
		}
	}
	if score > 100 {
		score = 100
	}
	result.RiskScore = score
	result.RiskLevel = strings.ToUpper(maxSeverity)

	return result
}
