package ml

import "testing"

func TestIsAgentCLIIncludesNewHarnesses(t *testing.T) {
	for _, comm := range []string{"dsh", "pi", "omp", "zg", "zcode", "ZCode"} {
		if !IsAgentCLI(comm) {
			t.Fatalf("IsAgentCLI(%q) = false", comm)
		}
	}
}
