package strictjson

import (
	"strings"
	"testing"
)

type example struct {
	Name   string    `json:"name"`
	Return *int64    `json:"return"`
	Nested []example `json:"nested"`
}

func TestAmbiguousJSON(t *testing.T) {
	for _, s := range []string{`{"name":"x","name":"y"}`, `{"name":"x","Name":"y"}`, `{"Name":"x"}`, `{"nested":[{"NAME":"x"}]}`, `{"name":"x"} {}`, `{"return":1.2}`, `{"return":9223372036854775808}`, `{"name":"` + string([]byte{255}) + `"}`, strings.Repeat(`{"nested":[`, 34) + `{}` + strings.Repeat(`]}`, 34)} {
		var v example
		if Decode([]byte(s), &v) == nil {
			t.Fatalf("ambiguous input accepted")
		}
	}
}
func TestPresenceAndUnicode(t *testing.T) {
	var v example
	if e := Decode([]byte(`{"name":"读","return":0,"nested":[]}`), &v); e != nil {
		t.Fatal(e)
	}
	if v.Return == nil || *v.Return != 0 {
		t.Fatal("lost zero presence")
	}
}
func FuzzStrictJSON(f *testing.F) {
	f.Add([]byte(`{"name":"read","return":0}`))
	f.Add([]byte(`{"Name":"x"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 65536 {
			t.Skip()
		}
		var v example
		_ = Decode(b, &v)
	})
}
