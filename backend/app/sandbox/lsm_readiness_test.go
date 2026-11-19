package sandbox

import "testing"

func TestBPFLSMConfiguredExactToken(t *testing.T) {
	for _, input := range []string{
		"lockdown,capability,landlock,bpf,apparmor",
		"bpf",
		"capability, bpf\n",
	} {
		if !bpfLSMConfigured(input) {
			t.Fatalf("active BPF LSM was not recognized: %q", input)
		}
	}
	for _, input := range []string{
		"", "lockdown,apparmor,landlock", "bpf_disabled",
		"mybpf", "capability,notbpf",
	} {
		if bpfLSMConfigured(input) {
			t.Fatalf("incorrectly treated an inactive BPF LSM as enabled: %q", input)
		}
	}
}
