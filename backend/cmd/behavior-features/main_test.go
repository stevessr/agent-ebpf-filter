package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIRejectsUnknownAndTrailing(t *testing.T) {
	for _, s := range []string{`{"parameter":"x","parameter":"y"}`, `{"syscall":{"name":"read","return":-1,"return":0}}`} {
		if run(strings.NewReader(s), &bytes.Buffer{}, false, false) == nil {
			t.Fatal("duplicate key accepted")
		}
	}
	for _, s := range []string{`{"parameter":"x","decision":"allow"}`, `{"parameter":"x"} {"parameter":"y"}`, `{"parameter":"x","risk_score":1}`} {
		var out bytes.Buffer
		if run(strings.NewReader(s), &out, false, false) == nil {
			t.Fatal("ambiguous or label input accepted")
		}
	}
}
func TestCLIStream(t *testing.T) {
	var out bytes.Buffer
	if e := run(strings.NewReader("{\"parameter\":\"x\"}\n{\"parameter\":\"y\"}\n"), &out, false, false); e != nil {
		t.Fatal(e)
	}
	if strings.Count(out.String(), "\n") != 2 {
		t.Fatal("missing output")
	}
}
