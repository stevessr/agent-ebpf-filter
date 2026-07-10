package protocoldetect

const (
	// These values are part of the REST/WebSocket presentation contract. Keep
	// their canonical protocol spelling instead of normalizing them to lower
	// case; flow enrichment and the frontend both expose them verbatim.
	AppProtoTLS     AppProtocol = "TLS"
	AppProtoHTTP    AppProtocol = "HTTP"
	AppProtoSSH     AppProtocol = "SSH"
	AppProtoDNS     AppProtocol = "DNS"
	AppProtoQUIC    AppProtocol = "QUIC"
	AppProtoDHCP    AppProtocol = "DHCP"
	AppProtomDNS    AppProtocol = "mDNS"
	AppProtoLLMNR   AppProtocol = "LLMNR"
	AppProtoSSDP    AppProtocol = "SSDP"
	AppProtoNTP     AppProtocol = "NTP"
	AppProtoSNMP    AppProtocol = "SNMP"
	AppProtoNetBIOS AppProtocol = "NetBIOS"
	AppProtoUnknown AppProtocol = "Unknown"
)
