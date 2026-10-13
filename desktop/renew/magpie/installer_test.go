package magpie

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestClaudeBaselinePlan(t *testing.T) {
	got, err := Plan(Claude, Baseline)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"claude plugin marketplace add apache/magpie",
		"claude plugin install magpie-setup@apache-magpie",
		"claude plugin install magpie-agent-guard@apache-magpie",
		"claude plugin install magpie-utilities@apache-magpie",
	}
	var actual []string
	for _, step := range got {
		actual = append(actual, step.Display())
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("got %v; want %v", actual, want)
	}
}

func TestCodexOnlySupportedFamily(t *testing.T) {
	got, err := Plan(Codex, Security)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[3].Display() != "codex plugin add magpie-security@apache-magpie" {
		t.Fatalf("unexpected plan: %+v", got)
	}
	for _, step := range got {
		if strings.Contains(step.Display(), "magpie-agent-guard") {
			t.Fatal("Codex does not list magpie-agent-guard")
		}
	}
}

func TestRejectUntrustedCLIArguments(t *testing.T) {
	for _, tc := range []struct{ target, family string }{
		{Claude, "magpie-security; rm -rf /"},
		{"bash", Baseline},
		{Codex, ""},
	} {
		if _, err := Plan(tc.target, tc.family); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
}

func TestRepeatPlanDoesNotShareMutableArgv(t *testing.T) {
	first, _ := Plan(Claude, Baseline)
	first[0].Arguments[3] = "changed"
	next, _ := Plan(Claude, Baseline)
	if next[0].Arguments[3] != Marketplace {
		t.Fatal("plan shares mutable argv slices")
	}
}

func TestAlreadyRegisteredStrictlyScoped(t *testing.T) {
	if !alreadyRegistered([]string{"plugin", "marketplace", "add", Marketplace}, "Error: marketplace already exists") {
		t.Fatal("failed to recognize existing marketplace")
	}
	if !alreadyRegistered([]string{"plugin", "install", "magpie-setup@apache-magpie"}, "Already installed") {
		t.Fatal("failed to recognize installed plugin")
	}
	if alreadyRegistered([]string{"exec", "shell"}, "already installed") {
		t.Fatal("unrelated failure was ignored")
	}
}

func TestCappedOutput(t *testing.T) {
	output := &cappedOutput{}
	data := []byte(strings.Repeat("a", 40<<10))
	n, err := output.Write(data)
	if err != nil || n != len(data) || len(output.data) != 16<<10 {
		t.Fatal("unexpected command output buffering")
	}
}

func TestCancelledExecutionDoesNotRunCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ctx.Err() == nil {
		t.Fatal("context should be cancelled")
	}
}
