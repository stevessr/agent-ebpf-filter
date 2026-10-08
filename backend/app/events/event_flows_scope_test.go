package events

import (
	"net"
	"testing"

	"agent-ebpf-filter/internal/network"
)

func TestClassifyFlowIPScopeMatchesNetPath(t *testing.T) {
	cases := []string{
		"local", "127.0.0.1", "0.0.0.0", "10.0.0.1", "100.64.1.2", "169.254.3.4",
		"172.16.5.6", "192.168.1.1", "192.0.2.9", "198.51.100.7", "203.0.113.9",
		"224.0.0.5", "240.1.2.3", "255.255.255.255", "93.184.216.34", "8.8.8.8",
		"1.1.1.1", "::1", "fe80::1", "2001:db8::68", "example.invalid", "",
		"256.1.1.1", "01.2.3.4", "1.2.3", "1.2.3.4.5", "1..2.3", ".1.2.3", "1.2.3.",
	}
	for _, ip := range cases {
		want := network.ClassifyIPScope(parseIPForScope(ip))
		if got := classifyFlowIPScope(ip); got != want {
			t.Fatalf("classifyFlowIPScope(%q) = %v, want %v", ip, got, want)
		}
	}
}

func parseIPForScope(ip string) net.IP {
	if ip == "local" {
		return net.ParseIP("127.0.0.1")
	}
	return net.ParseIP(ip)
}
