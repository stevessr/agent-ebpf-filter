package app

import "testing"

func TestDefaultTrackedCommandsOnlyAgentCLIEnabled(t *testing.T) {
	if len(defaultTrackedCommands) == 0 {
		t.Fatal("default tracked command registry is empty")
	}

	agentCLICommands := map[string]struct{}{}
	for comm, tag := range defaultTrackedCommands {
		enabled := defaultTrackedCommandEnabled(tag)
		if tag == defaultEnabledTrackedCommandTag {
			if !enabled {
				t.Fatalf("Agent CLI command %q is disabled by default", comm)
			}
			agentCLICommands[comm] = struct{}{}
			continue
		}
		if enabled {
			t.Fatalf("non-Agent CLI command %q with tag %q is enabled by default", comm, tag)
		}
	}

	for _, comm := range []string{"claude", "gemini", "codex", "kiro-cli", "gh", "cursor"} {
		if _, ok := agentCLICommands[comm]; !ok {
			t.Fatalf("expected default Agent CLI command %q to be enabled", comm)
		}
	}
}
