package main

import (
	"agent-ebpf-filter/internal/behaviorrisk"
	"bytes"
	"strings"
	"testing"
)

func TestMalformedAbstainsAndStreamContinues(t *testing.T) {
	var out, diag bytes.Buffer
	e := run(strings.NewReader("{\"Parameter\":\"bad\"}\n{\"parameter\":\"good\"}\n"), &out, &diag, &behaviorrisk.Manager{}, false)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Count(out.String(), "\n") != 2 || strings.Contains(out.String(), "good") || !strings.Contains(diag.String(), `"invalid":1`) {
		t.Fatal("bad stream handling")
	}
}
func TestHugeLineIsBounded(t *testing.T) {
	if run(strings.NewReader(strings.Repeat("x", 600000)), &bytes.Buffer{}, &bytes.Buffer{}, &behaviorrisk.Manager{}, false) == nil {
		t.Fatal("huge line accepted")
	}
}
