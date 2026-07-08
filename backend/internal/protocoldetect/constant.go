package protocoldetect

const (
	AppProtoTLS    AppProtocol = "tls"
	AppProtoHTTP   AppProtocol = "http"
	AppProtoSSH    AppProtocol = "ssh"
	AppProtoDNS    AppProtocol = "dns"
	AppProtoQUIC   AppProtocol = "quic"
	AppProtoDHCP   AppProtocol = "dhcp"
	AppProtomDNS   AppProtocol = "mdns"
	AppProtoLLMNR  AppProtocol = "llmnr"
	AppProtoSSDP   AppProtocol = "ssdp"
	AppProtoNTP    AppProtocol = "ntp"
	AppProtoSNMP   AppProtocol = "snmp"
	AppProtoNetBIOS AppProtocol = "netbios"
	AppProtoUnknown AppProtocol = "unknown"
)