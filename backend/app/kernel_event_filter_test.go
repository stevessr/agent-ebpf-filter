package app

import (
	"testing"
)

func withDisabledComms(t *testing.T, comms ...string) {
	t.Helper()
	disabledCommsMu.Lock()
	saved := disabledComms
	disabledComms = make(map[string]struct{}, len(comms))
	for _, comm := range comms {
		disabledComms[comm] = struct{}{}
	}
	disabledCommsMu.Unlock()
	t.Cleanup(func() {
		disabledCommsMu.Lock()
		disabledComms = saved
		disabledCommsMu.Unlock()
	})
}

func TestCommDisabledMatchesSanitizedComm(t *testing.T) {
	withDisabledComms(t, "node", "caf�", "ab")

	cases := map[string]struct {
		raw  string
		want bool
	}{
		"clean padded":            {raw: "node\x00\x00\x00\x00", want: true},
		"clean miss":              {raw: "bash\x00", want: false},
		"prefix does not match":   {raw: "nodejs\x00", want: false},
		"invalid utf8 sanitizes":  {raw: "caf\xff\x00", want: true},
		"embedded nul collapses":  {raw: "a\x00b\x00\x00", want: true},
		"empty buffer":            {raw: "\x00\x00", want: false},
		"exact fit without pad":   {raw: "node", want: true},
		"replacement char config": {raw: "caf\xe9", want: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var buf [16]byte
			copy(buf[:], tc.raw)
			if got := commDisabled(buf[:]); got != tc.want {
				t.Fatalf("commDisabled(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestCommDisabledDoesNotAllocateForCleanComm(t *testing.T) {
	withDisabledComms(t, "node")
	var buf [16]byte
	copy(buf[:], "claude-code")
	if allocs := testing.AllocsPerRun(1000, func() { commDisabled(buf[:]) }); allocs != 0 {
		t.Fatalf("commDisabled allocated %.1f times per call, want 0", allocs)
	}
}

func TestCommDisabledShortCircuitsWhenNothingDisabled(t *testing.T) {
	withDisabledComms(t)
	var buf [16]byte
	copy(buf[:], "node")
	if commDisabled(buf[:]) {
		t.Fatal("commDisabled reported a hit with an empty disable set")
	}
}

func TestSoakKernelMetadataEventOnlyAllowsIsolatedFixtureTypes(t *testing.T) {
	testCases := []struct {
		name string
		comm string
		kind uint32
		want bool
	}{
		{"fixture openat", "soak-file-opena", 1, true},
		{"fixture exec", "soak-file-opena", 26, true},
		{"fixture exit", "soak-file-opena", 28, true},
		{"fixture other event", "soak-file-opena", 25, false},
		{"other agent open", "claude", 1, false},
		{"similar prefix", "other-soak-file", 1, false},
		{"empty command", "", 1, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := &bpfEvent{Type: tc.kind}
			copy(event.Comm[:], tc.comm)
			if got := soakKernelMetadataEvent(event); got != tc.want {
				t.Fatalf("soakKernelMetadataEvent(comm=%q, type=%d) = %t, want %t",
					tc.comm, tc.kind, got, tc.want)
			}
		})
	}
	if soakKernelMetadataEvent(nil) {
		t.Fatal("nil event must not be logged")
	}
}
