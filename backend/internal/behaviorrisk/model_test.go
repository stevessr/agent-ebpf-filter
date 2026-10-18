package behaviorrisk

import (
	"agent-ebpf-filter/internal/behaviorfeatures"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
)

func ptr[T any](v T) *T { return &v }
func encoded(s Spec) ([]byte, string) {
	b, _ := json.Marshal(s)
	h := sha256.Sum256(b)
	return b, hex.EncodeToString(h[:])
}
func validSpec() Spec {
	return Spec{Encoding: Encoding, Kind: "http_parameter", Weights: make([]float64, Dimensions), Threshold: .5, ShadowOnly: true}
}
func TestModelIntegrityAndRollback(t *testing.T) {
	s := validSpec()
	s.Intercept = 2
	b, h := encoded(s)
	var m Manager
	if e := m.Reload(bytes.NewReader(b), h); e != nil {
		t.Fatal(e)
	}
	i := behaviorfeatures.Input{Parameter: ptr("innocuous test")}
	before := m.Score(i)
	if before.Status != "above_threshold" {
		t.Fatal(before)
	}
	if e := m.Reload(bytes.NewReader(b), strings.Repeat("0", 64)); e == nil {
		t.Fatal("checksum accepted")
	}
	after := m.Score(i)
	if before.ModelDigest != after.ModelDigest || *before.Score != *after.Score {
		t.Fatal("bad reload changed snapshot")
	}
	m.Disable()
	if m.Score(i).Score != nil {
		t.Fatal("disable did not abstain")
	}
}
func TestRejectMalformedModel(t *testing.T) {
	for _, mutate := range []func(*Spec){func(s *Spec) { s.ShadowOnly = false }, func(s *Spec) { s.Kind = "syscall" }, func(s *Spec) { s.Weights = s.Weights[:1] }, func(s *Spec) { s.Threshold = 1 }, func(s *Spec) { s.Encoding = "other" }, func(s *Spec) { s.Weights[0] = 1e7 }} {
		s := validSpec()
		mutate(&s)
		b, h := encoded(s)
		if _, e := Load(bytes.NewReader(b), h); e == nil {
			t.Fatal("invalid model accepted")
		}
	}
	if _, e := Load(strings.NewReader(strings.Repeat("x", MaxModelBytes+1)), strings.Repeat("0", 64)); e == nil {
		t.Fatal("unbounded model")
	}
}
func TestAbstentionAndNoSecrets(t *testing.T) {
	b, h := encoded(validSpec())
	m, e := Load(bytes.NewReader(b), h)
	if e != nil {
		t.Fatal(e)
	}
	for _, i := range []behaviorfeatures.Input{{Syscall: &behaviorfeatures.Syscall{Name: "read"}}, {HTTP: &behaviorfeatures.HTTP{Method: "GET"}}, {Parameter: ptr(strings.Repeat("a", MaxBytes+1))}, {Parameter: ptr("x"), CaptureLoss: ptr(true)}} {
		if m.Score(i).Score != nil {
			t.Fatal("uncertain input scored")
		}
	}
	secret := "SECRET_UNIQUE_684021"
	r := m.Score(behaviorfeatures.Input{Parameter: &secret})
	j, _ := json.Marshal(r)
	if strings.Contains(string(j), secret) {
		t.Fatal("secret output")
	}
}
func TestVectorDeterminismAndBounds(t *testing.T) {
	v, e := Encode("a%2520b SQL", "http_parameter")
	if e != nil {
		t.Fatal(e)
	}
	v2, _ := Encode("a%2520b SQL", "http_parameter")
	b, _ := json.Marshal(v)
	b2, _ := json.Marshal(v2)
	if !bytes.Equal(b, b2) {
		t.Fatal("nondeterministic")
	}
	sum := 0.
	last := -1
	for _, x := range v.Entries {
		if x.Index <= last || x.Index >= Dimensions || !finite(x.Value) {
			t.Fatal("vector invariant")
		}
		last = x.Index
		sum += x.Value * x.Value
	}
	if math.Abs(sum-1) > 1e-12 {
		t.Fatal(sum)
	}
}
func TestConcurrentReload(t *testing.T) {
	s := validSpec()
	b, h := encoded(s)
	var m Manager
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				if j%3 == 0 {
					if e := m.Reload(bytes.NewReader(b), h); e != nil {
						t.Error(e)
					}
				} else if j%3 == 1 {
					m.Score(behaviorfeatures.Input{Parameter: ptr("test")})
				} else {
					m.Disable()
				}
			}
		}()
	}
	wg.Wait()
}
func BenchmarkShadowScore(b *testing.B) {
	raw, h := encoded(validSpec())
	m, e := Load(bytes.NewReader(raw), h)
	if e != nil {
		b.Fatal(e)
	}
	i := behaviorfeatures.Input{Parameter: ptr("q=read+the+documentation&page=10")}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		m.Score(i)
	}
}
func FuzzBoundedVector(f *testing.F) {
	f.Add("hello%20world")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 65536 {
			t.Skip()
		}
		v, e := Encode(s, "http_parameter")
		if e == nil {
			for _, x := range v.Entries {
				if !finite(x.Value) || x.Index < 0 || x.Index >= Dimensions {
					t.Fatal("invalid vector")
				}
			}
		}
	})
}
