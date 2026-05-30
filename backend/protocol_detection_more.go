package main

import "agent-ebpf-filter/internal/protocoldetect"

func extractSSHInfo(data []byte) (version string, software string, err error) {
	return protocoldetect.ExtractSSHInfo(data)
}

func extractDHCPInfo(data []byte) (string, string, error) {
	return protocoldetect.ExtractDHCPInfo(data)
}

func extractDNSQueries(data []byte) []string {
	return protocoldetect.ExtractDNSQueries(data)
}

func extractMDNSQueries(data []byte) []string {
	return protocoldetect.ExtractMDNSQueries(data)
}

func extractQUICSNI(data []byte) string {
	return protocoldetect.ExtractQUICSNI(data)
}

func extractTLSSNIFromHandshake(data []byte) (string, string, error) {
	return protocoldetect.ExtractTLSSNIFromHandshake(data)
}

func extractQUICVersion(data []byte) string {
	return protocoldetect.ExtractQUICVersion(data)
}

func extractNTPInfo(data []byte) (version string, stratum string) {
	return protocoldetect.ExtractNTPInfo(data)
}

func extractSNMPInfo(data []byte) (version string, community string) {
	return protocoldetect.ExtractSNMPInfo(data)
}

func extractNetBIOSInfo(data []byte) (name string, nsType string) {
	return protocoldetect.ExtractNetBIOSInfo(data)
}
