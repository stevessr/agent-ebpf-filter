package events

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ---- kernel/user risk bridge ------------------------------------------------

type kernelRiskDecision struct {
	Decision string
	Score    float64
	Reasons  []string
}

type KernelRiskDecision = kernelRiskDecision

func (d kernelRiskDecision) reasonText() string {
	if len(d.Reasons) == 0 {
		return ""
	}
	return strings.Join(d.Reasons, ",")
}

// applyKernelRiskDecision runs a low-latency user-space risk pass while the
// eBPF ring-buffer sample is still being consumed. The event pointer is a
// zero-copy view over the mmap-backed sample in the fast path; do not retain it.
func ApplyKernelRiskDecision(raw *BpfEvent, event *pb.Event) {
	if raw == nil || event == nil {
		return
	}

	start := time.Now()
	decision := evaluateKernelRiskDecision(raw, event)
	Deps.CollectorMetrics.RecordKernelRiskDecision(decision.Decision, time.Since(start))
	if decision.Score <= 0 {
		return
	}

	if event.RiskScore < decision.Score {
		event.RiskScore = decision.Score
	}
	if event.Decision == "" && decision.Decision != "" {
		event.Decision = decision.Decision
	}
	if reason := decision.reasonText(); reason != "" {
		riskInfo := fmt.Sprintf("kernel_risk score=%.0f decision=%s reasons=%s", decision.Score, Deps.StringsTrimDefault(decision.Decision, "OBSERVE"), reason)
		if event.ExtraInfo == "" {
			event.ExtraInfo = riskInfo
		} else if !strings.Contains(event.ExtraInfo, "kernel_risk score=") {
			event.ExtraInfo = event.ExtraInfo + " " + riskInfo
		}
	}
	queueKernelRiskFeedback(event, decision)
}

func evaluateKernelRiskDecision(raw *BpfEvent, event *pb.Event) kernelRiskDecision {
	var out kernelRiskDecision
	seenReasons := make(map[string]struct{}, 6)
	add := func(points float64, reason string) {
		if points <= 0 {
			return
		}
		out.Score += points
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return
		}
		if _, ok := seenReasons[reason]; ok {
			return
		}
		seenReasons[reason] = struct{}{}
		if len(out.Reasons) < 6 {
			out.Reasons = append(out.Reasons, reason)
		}
	}

	typeName := event.GetType()
	comm := strings.ToLower(event.GetComm())
	path := strings.ToLower(platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath()))
	tag := strings.ToLower(event.GetTag())

	if strings.Contains(tag, "agent") || strings.Contains(tag, "wrapper") {
		add(8, "agent_context")
	}

	switch typeName {
	case "unlink", "unlinkat", "rename":
		add(24, "destructive_file_mutation")
	case "chmod", "chown", "mknod":
		add(20, "permission_or_device_mutation")
	case "link", "symlink":
		add(16, "link_mutation")
	case "write":
		add(12, "file_write")
	case "open", "openat", "read":
		if isSensitiveKernelRiskPath(path) {
			add(12, "sensitive_file_access")
		}
	case "execve", "process_exec":
		add(8, "process_exec")
		if isTmpExecutableKernelRiskPath(path) {
			add(14, "tmp_exec")
		}
	case "ioctl":
		add(12, "ioctl")
	case "socket":
		if event.GetSockType() == "SOCK_RAW" || raw.Extra2 == 3 {
			add(26, "raw_socket")
		}
	case "network_connect", "network_sendto", "tcp_connect", "dns_query":
		add(16, "network_egress")
	}

	if isSensitiveKernelRiskPath(path) {
		add(34, "sensitive_path")
	}
	if isSecretKernelRiskPath(path) {
		add(30, "secret_material_path")
	}
	if isSuspiciousKernelRiskComm(comm) {
		add(12, "dual_use_comm")
	}

	host, port := kernelRiskEndpoint(event)
	if host != "" {
		if ip := net.ParseIP(host); ip != nil && !isPrivateKernelRiskIP(ip) {
			add(16, "public_network_endpoint")
		}
		if isHighRiskKernelPort(port) {
			add(18, "high_risk_port_"+strconv.FormatUint(uint64(port), 10))
		}
		if isSuspiciousKernelRiskComm(comm) {
			add(10, "dual_use_network_tool")
		}
	}

	if raw.Retval < 0 && out.Score > 0 {
		// Failed attempts still matter, but should score slightly lower than a
		// successful side effect.
		out.Score *= 0.8
		add(4, "failed_attempt")
	}

	if out.Score > 100 {
		out.Score = 100
	}
	switch {
	case out.Score >= 60:
		out.Decision = "ALERT"
	case out.Score >= 35:
		out.Decision = "OBSERVE"
	default:
		out.Decision = ""
	}
	return out
}

func kernelRiskEndpoint(event *pb.Event) (string, uint32) {
	if event == nil {
		return "", 0
	}
	host := strings.TrimSpace(event.GetDstIp())
	port := event.GetDstPort()
	if host != "" || port > 0 {
		return host, port
	}
	endpoint := strings.TrimSpace(event.GetNetEndpoint())
	if endpoint == "" {
		return "", 0
	}
	if before, _, ok := strings.Cut(endpoint, " "); ok {
		endpoint = before
	}
	if h, p, err := net.SplitHostPort(endpoint); err == nil {
		parsed, _ := strconv.ParseUint(p, 10, 32)
		return strings.Trim(h, "[]"), uint32(parsed)
	}
	if strings.HasPrefix(endpoint, "dns:") {
		return "", 53
	}
	if idx := strings.LastIndex(endpoint, ":"); idx > 0 {
		parsed, _ := strconv.ParseUint(endpoint[idx+1:], 10, 32)
		return strings.Trim(endpoint[:idx], "[]"), uint32(parsed)
	}
	return endpoint, 0
}

func isSensitiveKernelRiskPath(path string) bool {
	if path == "" {
		return false
	}
	cleaned := filepath.Clean(path)
	return strings.HasPrefix(cleaned, "/etc/") ||
		strings.HasPrefix(cleaned, "/root/") ||
		strings.HasPrefix(cleaned, "/home/") && strings.Contains(cleaned, "/.ssh/") ||
		strings.HasPrefix(cleaned, "/proc/kcore") ||
		strings.HasPrefix(cleaned, "/proc/sys/") ||
		strings.HasPrefix(cleaned, "/sys/kernel/security") ||
		strings.HasPrefix(cleaned, "/var/run/docker.sock") ||
		strings.HasPrefix(cleaned, "/run/docker.sock")
}

func isSecretKernelRiskPath(path string) bool {
	base := filepath.Base(path)
	switch base {
	case "shadow", "gshadow", "passwd", "sudoers", "authorized_keys", "id_rsa", "id_ed25519", ".env", "credentials", "config.json":
		return true
	default:
		return strings.Contains(path, "private_key") ||
			strings.Contains(path, "api_key") ||
			strings.Contains(path, "token") ||
			strings.Contains(path, "secret")
	}
}

func isTmpExecutableKernelRiskPath(path string) bool {
	if path == "" {
		return false
	}
	cleaned := filepath.Clean(path)
	return strings.HasPrefix(cleaned, "/tmp/") ||
		strings.HasPrefix(cleaned, "/var/tmp/") ||
		strings.HasPrefix(cleaned, "/dev/shm/")
}

func isSuspiciousKernelRiskComm(comm string) bool {
	switch filepath.Base(comm) {
	case "curl", "wget", "nc", "ncat", "netcat", "socat", "ssh", "scp", "bash", "sh", "python", "python3", "perl", "ruby", "node", "openssl":
		return true
	default:
		return false
	}
}

func isHighRiskKernelPort(port uint32) bool {
	switch port {
	case 22, 23, 53, 135, 139, 389, 445, 1433, 1521, 2049, 2375, 2376, 3306, 3389, 4444, 5432, 6379, 9200, 11211, 27017:
		return true
	default:
		return port >= 60000
	}
}

func isPrivateKernelRiskIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
