package app

import "testing"

func TestKernelCommLookupKeyMatchesSanitizerOnFastPath(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "nul padded", raw: []byte{'b', 'a', 's', 'h', 0, 0, 0, 0}},
		{name: "full buffer", raw: []byte("abcdefghijklmnop")},
		{name: "empty", raw: []byte{0, 0, 0, 0}},
		{name: "utf8", raw: append([]byte("鱼鱼"), 0, 0, 0, 0)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, fast := kernelCommLookupKey(test.raw)
			if !fast {
				t.Fatal("expected fast path")
			}
			if want := sanitizeUTF8(test.raw); key != want {
				t.Fatalf("key = %q, want %q", key, want)
			}
		})
	}
}

func TestKernelCommLookupKeyFallsBackForNonCanonicalInput(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "embedded nul", raw: []byte{'a', 0, 'b', 0}},
		{name: "invalid utf8", raw: []byte{'a', 0xff, 0, 0}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if key, fast := kernelCommLookupKey(test.raw); fast {
				t.Fatalf("unexpected fast path with key %q", key)
			}
		})
	}
}

func TestIsRawCommDisabledPreservesFallbackSemantics(t *testing.T) {
	disabledCommsMu.Lock()
	previous := disabledComms
	disabledComms = map[string]struct{}{
		"bash": {},
		"ab":   {},
		"a�":   {},
	}
	disabledCommsMu.Unlock()
	defer func() {
		disabledCommsMu.Lock()
		disabledComms = previous
		disabledCommsMu.Unlock()
	}()

	cases := []struct {
		name string
		raw  []byte
		want bool
	}{
		{name: "canonical", raw: []byte{'b', 'a', 's', 'h', 0, 0}, want: true},
		{name: "embedded nul fallback", raw: []byte{'a', 0, 'b', 0}, want: true},
		{name: "invalid utf8 fallback", raw: []byte{'a', 0xff, 0}, want: true},
		{name: "not disabled", raw: []byte{'s', 'h', 0, 0}, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := isRawCommDisabled(test.raw); got != test.want {
				t.Fatalf("disabled = %t, want %t", got, test.want)
			}
		})
	}

	raw := []byte{'b', 'a', 's', 'h', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if allocs := testing.AllocsPerRun(1000, func() { _ = isRawCommDisabled(raw) }); allocs != 0 {
		t.Fatalf("canonical comm lookup allocated %.2f objects/run, want 0", allocs)
	}
}

func BenchmarkKernelCommFilterFastPath(b *testing.B) {
	disabledCommsMu.Lock()
	previous := disabledComms
	disabledComms = map[string]struct{}{"bash": {}}
	disabledCommsMu.Unlock()
	defer func() {
		disabledCommsMu.Lock()
		disabledComms = previous
		disabledCommsMu.Unlock()
	}()

	raw := []byte{'b', 'a', 's', 'h', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = isRawCommDisabled(raw)
	}
}

func BenchmarkKernelCommFilterSanitizedBaseline(b *testing.B) {
	disabledCommsMu.Lock()
	previous := disabledComms
	disabledComms = map[string]struct{}{"bash": {}}
	disabledCommsMu.Unlock()
	defer func() {
		disabledCommsMu.Lock()
		disabledComms = previous
		disabledCommsMu.Unlock()
	}()

	raw := []byte{'b', 'a', 's', 'h', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		comm := sanitizeUTF8(raw)
		disabledCommsMu.RLock()
		_, _ = disabledComms[comm]
		disabledCommsMu.RUnlock()
	}
}
