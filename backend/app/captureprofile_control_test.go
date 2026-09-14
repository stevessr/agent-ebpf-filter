package app

import (
	"agent-ebpf-filter/app/captureprofile"
	"path/filepath"
	"testing"
)

func TestCaptureProfileOverlayPersistsAndPublishes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	t.Setenv("AGENT_EBPF_API_PROFILES", path)
	defer func() {
		_ = captureprofile.Default.Replace(captureprofile.BuiltinProfiles())
	}()

	profiles := []captureprofile.Profile{{
		ID: "custom.acme.jobs", Vendor: "acme", Product: "jobs", Operation: "create",
		Sources: []string{"kernel_socket_prefix"}, Protocols: []string{"http1"},
		Directions: []string{"outgoing"}, Methods: []string{"POST"},
		HostSuffixes: []string{"api.acme.test"}, PathPrefixes: []string{"/v9/jobs"}, MinScore: 90,
	}}
	if err := persistCaptureProfileOverlay(path, profiles); err != nil {
		t.Fatal(err)
	}
	state, err := buildCaptureProfileState(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.CustomCount != 1 || state.CustomProfiles[0].ID != "custom.acme.jobs" {
		t.Fatalf("unexpected custom profile state: %+v", state)
	}
	match, ok := captureprofile.Default.Match(captureprofile.Observation{
		Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing",
		Method: "POST", Host: "api.acme.test", Path: "/v9/jobs/42",
	})
	if !ok || match.ProfileID != "custom.acme.jobs" {
		t.Fatalf("published registry did not match custom profile: ok=%v match=%+v", ok, match)
	}
}

func TestPreviewCaptureProfileReturnsMatchingIndexes(t *testing.T) {
	profile := captureprofile.Profile{
		ID: "custom.webhook", Vendor: "acme", Sources: []string{"kernel_socket_prefix"},
		Protocols: []string{"http1"}, Directions: []string{"incoming"},
		Methods: []string{"POST"}, HostSuffixes: []string{"hooks.acme.test"}, MinScore: 70,
	}
	observations := []captureprofile.Observation{
		{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Host: "hooks.acme.test"},
		{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "incoming", Method: "POST", Host: "hooks.acme.test"},
		{Source: "tls_plaintext", Protocol: "http1", Direction: "incoming", Method: "POST", Host: "hooks.acme.test"},
	}
	response, err := previewCaptureProfile(profile, observations)
	if err != nil {
		t.Fatal(err)
	}
	if response.MatchedCount != 1 || len(response.Matches) != 1 || response.Matches[0].Index != 1 {
		t.Fatalf("unexpected preview response: %+v", response)
	}
}

func TestCaptureProfileOverlayPathUsesConfiguredPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed-profiles.json")
	t.Setenv("AGENT_EBPF_API_PROFILES", path)
	if got := captureProfileOverlayPath(); got != path {
		t.Fatalf("overlay path = %q, want %q", got, path)
	}
}
