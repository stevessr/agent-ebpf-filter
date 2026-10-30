package main

import "testing"

func TestReviewedPortInput(t *testing.T) {
	if _, err := parsePorts(""); err == nil {
		t.Fatal("missing explicit review scope accepted")
	}
	if _, err := parsePorts("22,22"); err == nil {
		t.Fatal("duplicate port accepted")
	}
	if _, err := parsePorts("0"); err == nil {
		t.Fatal("port zero accepted")
	}
	if _, err := parsePorts("65536"); err == nil {
		t.Fatal("out-of-range port accepted")
	}
	if ports, err := parsePorts("22,4444"); err != nil || len(ports) != 2 || ports[0] != 22 || ports[1] != 4444 {
		t.Fatalf("valid reviewed ports rejected: %v / %v", ports, err)
	}
}

func TestRootCgroupAlwaysRejected(t *testing.T) {
	if _, err := scopeCgroup("/"); err == nil {
		t.Fatal("host root cgroup scope accepted")
	}
}
