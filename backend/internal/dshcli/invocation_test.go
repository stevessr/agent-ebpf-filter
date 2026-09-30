package dshcli

import "testing"

func TestParsePreservesDshLauncherBoundary(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want Invocation
	}{
		{name: "profile shorthand", args: []string{"web"}, want: Invocation{Mode: ModeProfile, Profile: "web"}},
		{name: "explicit profile", args: []string{"--profile", "web", "--port", "8080"}, want: Invocation{Mode: ModeProfile, Profile: "web"}},
		{name: "headless app suffix is opaque", args: []string{"headless", "  keep spacing  ", "--dump-config"}, want: Invocation{Mode: ModeProfile, Profile: "headless"}},
		{name: "dump config", args: []string{"--profile", "web", "--dump-config"}, want: Invocation{Mode: ModeDumpConfig, Profile: "web"}},
		{name: "dump default config", args: []string{"web", "--dump-default-config"}, want: Invocation{Mode: ModeDumpDefaultConfig, Profile: "web"}},
		{name: "schema after template", args: []string{"rescue", "--from-default-profile", "web", "--dump-config-schema"}, want: Invocation{Mode: ModeDumpConfigSchema, Profile: "rescue"}},
		{name: "launcher help", args: []string{"--help"}, want: Invocation{Mode: ModeLauncher}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse(tt.args); got != tt.want {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParsePluginKeepsOnlySafeOperationMetadata(t *testing.T) {
	tests := []struct {
		args []string
		want Invocation
	}{
		{args: []string{"plugin", "--profile", "tui", "add", "@scope/private-package"}, want: Invocation{Mode: ModePlugin, Profile: "tui", Operation: "add"}},
		{args: []string{"plugin", "--profile=web", "allow-version", "pkg@1.2.3", "--dsh-version", "0.2.0", "--accept-risk"}, want: Invocation{Mode: ModePlugin, Profile: "web", Operation: "allow-version"}},
		{args: []string{"plugin", "--profile", "tui", "run", "secret-script"}, want: Invocation{Mode: ModePlugin, Profile: "tui", Operation: "pnpm"}},
	}
	for _, tt := range tests {
		if got := Parse(tt.args); got != tt.want {
			t.Fatalf("Parse(%q) = %#v, want %#v", tt.args, got, tt.want)
		}
	}
}

func TestIsCommandAcceptsResolvedDshPath(t *testing.T) {
	if !IsCommand("/usr/local/bin/dsh") {
		t.Fatal("expected dsh path to be recognized")
	}
	if IsCommand("node") {
		t.Fatal("node must not be recognized as dsh")
	}
}
