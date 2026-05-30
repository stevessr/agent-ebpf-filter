package main

import "agent-ebpf-filter/internal/protocoldetect"

type HTTPRequestInfo = protocoldetect.HTTPRequestInfo
type AppProtocol = protocoldetect.AppProtocol

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

func extractTLSSNI(data []byte) (string, string, error) {
	return protocoldetect.ExtractTLSSNI(data)
}

func extractHTTPRequest(data []byte) (*HTTPRequestInfo, error) {
	return protocoldetect.ExtractHTTPRequest(data)
}

func fingerprintProtocol(data []byte, dport uint32) AppProtocol {
	return protocoldetect.FingerprintProtocol(data, dport)
}
