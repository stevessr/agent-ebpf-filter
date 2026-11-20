package events

import (
	"testing"

	"agent-ebpf-filter/pb"
)

func TestSemanticAnomalyExactCommAndEndpointMatching(t *testing.T) {
	// Benign tools whose names happen to contain "nc" or start with "ssh" must
	// not trigger false-positive process alerts.
	for _, tc := range []struct {
		name  string
		event *pb.Event
	}{
		{
			name:  "devbox_rsync",
			event: &pb.Event{ToolName: "remote_sync", Type: "execve", Comm: "rsync", Path: "/usr/bin/rsync"},
		},
		{
			name:  "devbox_sync",
			event: &pb.Event{ToolName: "devbox", Type: "process_exec", Comm: "sync", Path: "/bin/sync"},
		},
		{
			name:  "browser_ncurses",
			event: &pb.Event{ToolName: "playwright", Type: "execve", Comm: "ncurses6-config", Path: "/usr/bin/ncurses6-config"},
		},
		{
			name:  "browser_sshd_agent",
			event: &pb.Event{ToolName: "browser_open", Type: "execve", Comm: "ssh-agent", Path: "/usr/bin/ssh-agent"},
		},
	} {
		if reason, hit := detectRemoteDevboxAnomaly(tc.event); hit {
			t.Fatalf("%s falsely hit remote devbox anomaly: %s", tc.name, reason)
		}
		if reason, hit := detectBrowserTaskAnomaly(tc.event); hit {
			t.Fatalf("%s falsely hit browser task anomaly: %s", tc.name, reason)
		}
	}

	// Genuine risky executables must still trigger cleanly.
	for _, comm := range []string{"nc", "ncat", "socat"} {
		ev := &pb.Event{ToolName: "remote_exec", Type: "execve", Comm: comm, Path: "/usr/bin/" + comm}
		if _, hit := detectRemoteDevboxAnomaly(ev); !hit {
			t.Fatalf("devbox %q should trigger anomaly", comm)
		}
		bev := &pb.Event{ToolName: "browser_ui", Type: "execve", Comm: comm, Path: "/usr/bin/" + comm}
		if _, hit := detectBrowserTaskAnomaly(bev); !hit {
			t.Fatalf("browser %q should trigger anomaly", comm)
		}
	}

	// Endpoint classification: loopback IPv4/IPv6 exempt, spoofed hostnames flagged,
	// and exact port matching avoids substring false positives like :44440.
	if !isLocalhostOrLoopbackEndpoint("127.0.0.1:8080") || !isLocalhostOrLoopbackEndpoint("[::1]:8080") {
		t.Fatal("loopback endpoints must be recognized as local")
	}
	if isLocalhostOrLoopbackEndpoint("127.0.0.1.evil.com:443") || isLocalhostOrLoopbackEndpoint("evil-localhost.com:443") {
		t.Fatal("spoofed localhost-like hostnames must not be treated as loopback")
	}
	prSpoofed := &pb.Event{ToolName: "pr_review", Type: "network_connect", NetEndpoint: "127.0.0.1.evil.com:443"}
	if _, hit := detectPRReviewAnomaly(prSpoofed); !hit {
		t.Fatal("PR review egress to spoofed localhost-like domain must trigger anomaly")
	}
	prLocal := &pb.Event{ToolName: "pr_review", Type: "network_connect", NetEndpoint: "[::1]:8080"}
	if reason, hit := detectPRReviewAnomaly(prLocal); hit {
		t.Fatalf("PR review IPv6 loopback falsely triggered: %s", reason)
	}
	if isSuspiciousEndpoint("service.internal:44440") {
		t.Fatal("port 44440 must not substring-match 4444")
	}
	if !isSuspiciousEndpoint("attacker.ngrok.io:443") || !isSuspiciousEndpoint("203.0.113.10:4444") {
		t.Fatal("expected ngrok / port 4444 endpoint to trigger suspicious classification")
	}
}
