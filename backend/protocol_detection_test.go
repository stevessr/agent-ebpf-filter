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
