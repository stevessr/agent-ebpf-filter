package ml

import "testing"

func TestIsAgentCLIIncludesNewHarnesses(t *testing.T) {
	for _, comm := range []string{"dsh", "pi", "omp"} {
		if !IsAgentCLI(comm) {
			t.Fatalf("IsAgentCLI(%q) = false", comm)
		}
	}
}
