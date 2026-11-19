package app

import "testing"

func TestResearchAttributionTracksAllCompetingToolComms(t *testing.T) {
	for _, mode := range []string{"pid-only", "lifecycle-only", "combined"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT_EBPF_RESEARCH_ATTRIBUTION_MODE", mode)
			for _, comm := range []string{"git", "python", "python3", "node"} {
				if !researchAttributionTracksCommand(comm) {
					t.Fatalf("research mode %q silently disabled competing comm %q", mode, comm)
				}
			}
			for _, comm := range []string{"ssh", "bash", "unknown"} {
				if researchAttributionTracksCommand(comm) {
					t.Fatalf("research mode %q enabled unrelated comm %q", mode, comm)
				}
			}
		})
	}
}

func TestResearchAttributionNeverChangesProductionCommDefaults(t *testing.T) {
	for _, mode := range []string{"", "unrecognized", "off"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT_EBPF_RESEARCH_ATTRIBUTION_MODE", mode)
			for _, comm := range []string{"git", "python", "python3", "node"} {
				if researchAttributionTracksCommand(comm) {
					t.Fatalf("default/invalid mode unexpectedly changed production comm filtering: mode=%q comm=%q", mode, comm)
				}
			}
		})
	}
}
