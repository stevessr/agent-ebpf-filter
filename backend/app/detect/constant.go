package detect

import "agent-ebpf-filter/internal/protocoldetect"

const (
	AppProtoTLS     = protocoldetect.AppProtoTLS
	AppProtoHTTP    = protocoldetect.AppProtoHTTP
	AppProtoSSH     = protocoldetect.AppProtoSSH
	AppProtoDNS     = protocoldetect.AppProtoDNS
	AppProtoQUIC    = protocoldetect.AppProtoQUIC
	AppProtoDHCP    = protocoldetect.AppProtoDHCP
	AppProtomDNS    = protocoldetect.AppProtomDNS
	AppProtoLLMNR   = protocoldetect.AppProtoLLMNR
	AppProtoSSDP    = protocoldetect.AppProtoSSDP
	AppProtoNTP     = protocoldetect.AppProtoNTP
	AppProtoSNMP    = protocoldetect.AppProtoSNMP
	AppProtoNetBIOS = protocoldetect.AppProtoNetBIOS
	AppProtoUnknown = protocoldetect.AppProtoUnknown
)