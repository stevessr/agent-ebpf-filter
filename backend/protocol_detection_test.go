package main

import "testing"

func TestExtractDNSQueries(t *testing.T) {
	query := []byte{
		0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x03, 'c', 'o', 'm', 0x00,
		0x00, 0x01, 0x00, 0x01,
	}
	queries := extractDNSQueries(query)
	if len(queries) != 1 || queries[0] != "example.com" {
		t.Fatalf("queries = %#v, want example.com", queries)
	}
	entry := detectAndRecordProtocol("8.8.8.8", 53, query)
	if entry == nil || entry.AppProtocol != AppProtoDNS || entry.HTTPHost != "example.com" {
		t.Fatalf("entry = %#v, want DNS example.com", entry)
	}
}

func TestCorrelateDNSResponseRecordsAnswers(t *testing.T) {
	orig := dnsCorrelation
	dnsCorrelation = newDNSCache()
	defer func() { dnsCorrelation = orig }()

	response := []byte{
		0x12, 0x34, 0x81, 0x80, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x03, 'c', 'o', 'm', 0x00,
		0x00, 0x01, 0x00, 0x01,
		0xc0, 0x0c, 0x00, 0x01, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x3c,
		0x00, 0x04, 93, 184, 216, 34,
	}
	correlateDNSResponse("8.8.8.8", response)
	domain, ok := dnsCorrelation.LookupIP("93.184.216.34")
	if !ok || domain != "example.com" {
		t.Fatalf("LookupIP = %q %v, want example.com true", domain, ok)
	}
	ip, ok := dnsCorrelation.LookupDomain("example.com")
	if !ok || ip != "93.184.216.34" {
		t.Fatalf("LookupDomain = %q %v, want 93.184.216.34 true", ip, ok)
	}
}

func TestFingerprintQUIC(t *testing.T) {
	quicInitial := []byte{
		0xc0, 0x00, 0x00, 0x00, 0x01,
		0x08, 1, 2, 3, 4, 5, 6, 7, 8,
		0x00,
	}
	if proto := fingerprintProtocol(quicInitial, 443); proto != AppProtoQUIC {
		t.Fatalf("fingerprint = %q, want QUIC", proto)
	}
}

func TestFingerprintNTP(t *testing.T) {
	ntp := make([]byte, 48)
	ntp[0] = 0x23
	if proto := fingerprintProtocol(ntp, 123); proto != AppProtoNTP {
		t.Fatalf("fingerprint NTP = %q, want NTP", proto)
	}
	ver, stratum := extractNTPInfo(ntp)
	if ver == "" || stratum == "" {
		t.Fatalf("extractNTPInfo empty")
	}
}

func TestFingerprintSNMP(t *testing.T) {
	snmp := []byte{
		0x30, 0x26, 0x02, 0x01, 0x01,
		0x04, 0x06, 'p', 'u', 'b', 'l', 'i', 'c',
	}
	if proto := fingerprintProtocol(snmp, 161); proto != AppProtoSNMP {
		t.Fatalf("fingerprint SNMP = %q", proto)
	}
	ver, comm := extractSNMPInfo(snmp)
	if ver != "SNMPv2c" || comm != "public" {
		t.Fatalf("extractSNMPInfo = %q %q", ver, comm)
	}
}

func TestFingerprintSSDP(t *testing.T) {
	ssdp := []byte("NOTIFY * HTTP/1.1\r\nHost: 239.255.255.250:1900\r\n")
	if proto := fingerprintProtocol(ssdp, 1900); proto != AppProtoSSDP {
		t.Fatalf("fingerprint SSDP = %q", proto)
	}
}

func TestFingerprintNetBIOS(t *testing.T) {
	nbns := make([]byte, 50)
	nbns[2], nbns[3] = 0x00, 0x00
	nbns[4], nbns[5] = 0x00, 0x01
	nbns[12] = 0x20
	nameBytes := []byte("WORKGROUP")
	for i := 0; i < 16; i++ {
		c := byte('A')
		if i < len(nameBytes) {
			c = nameBytes[i]
		}
		nbns[13+i*2] = 'A' + (c>>4)&0x0f
		nbns[13+i*2+1] = 'A' + c&0x0f
	}
	if proto := fingerprintProtocol(nbns, 137); proto != AppProtoNetBIOS {
		t.Fatalf("fingerprint NetBIOS = %q", proto)
	}
}

func TestFingerprintLLMNR(t *testing.T) {
	llmnr := make([]byte, 12)
	llmnr[4], llmnr[5] = 0x00, 0x01
	if proto := fingerprintProtocol(llmnr, 5355); proto != AppProtoLLMNR {
		t.Fatalf("fingerprint LLMNR = %q", proto)
	}
}

func TestQUICVersion(t *testing.T) {
	v1 := extractQUICVersion([]byte{0xc0, 0x00, 0x00, 0x00, 0x01})
	if v1 != "QUIC v1" {
		t.Fatalf("QUIC version = %q", v1)
	}
}
