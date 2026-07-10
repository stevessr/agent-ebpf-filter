package protocoldetect

import "testing"

func TestAppProtocolCanonicalLabels(t *testing.T) {
	tests := map[AppProtocol]string{
		AppProtoTLS:     "TLS",
		AppProtoHTTP:    "HTTP",
		AppProtoSSH:     "SSH",
		AppProtoDNS:     "DNS",
		AppProtoQUIC:    "QUIC",
		AppProtoDHCP:    "DHCP",
		AppProtomDNS:    "mDNS",
		AppProtoLLMNR:   "LLMNR",
		AppProtoSSDP:    "SSDP",
		AppProtoNTP:     "NTP",
		AppProtoSNMP:    "SNMP",
		AppProtoNetBIOS: "NetBIOS",
		AppProtoUnknown: "Unknown",
	}

	for protocol, want := range tests {
		if got := string(protocol); got != want {
			t.Errorf("protocol label = %q, want %q", got, want)
		}
	}
}
