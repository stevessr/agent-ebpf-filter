package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSandboxOptionsRejectUnsafeModes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		opts      sandboxOptions
		wantError bool
	}{
		{name: "legacy off", opts: sandboxOptions{Mode: "off"}},
		{name: "readonly", opts: sandboxOptions{Mode: "readonly"}},
		{name: "workspace", opts: sandboxOptions{Mode: "workspace"}},
		{name: "unknown", opts: sandboxOptions{Mode: "auto"}, wantError: true},
		{name: "network without sandbox", opts: sandboxOptions{Mode: "off", Network: true}, wantError: true},
		{name: "bind without sandbox", opts: sandboxOptions{Mode: "off", ReadOnly: []string{"/etc"}}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.validate()
			if (err != nil) != tc.wantError {
				t.Fatalf("validate() error = %v; wantError=%v", err, tc.wantError)
			}
		})
	}
}

func TestSandboxContainerIDUnique(t *testing.T) {
	one, err := newSandboxContainerID()
	if err != nil {
		t.Fatal(err)
	}
	two, err := newSandboxContainerID()
	if err != nil {
		t.Fatal(err)
	}
	if one == two || !strings.HasPrefix(one, "wrapper-bwrap-") || len(one) < 30 {
		t.Fatalf("sandbox ids must be unique and namespaced: %q %q", one, two)
	}
}

func TestSandboxPathsAndBinds(t *testing.T) {
	if within("/tmp/other", "/tmp/project") || within("/tmp/project-x", "/tmp/project") {
		t.Fatal("path comparison must not allow sibling directories")
	}
	if !within("/tmp/project/file", "/tmp/project") || !within("/tmp/project", "/tmp/project") {
		t.Fatal("path comparison must admit descendants and the root itself")
	}
	if got, want := targetParents("/tmp/a/b/file"), []string{"/tmp", "/tmp/a", "/tmp/a/b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("targetParents=%v, want=%v", got, want)
	}
	var binds sandboxBinds
	if err := binds.Set(""); err == nil {
		t.Fatal("empty bind must be rejected")
	}
	if err := binds.Set("/opt/share"); err != nil || binds.String() != "/opt/share" {
		t.Fatalf("binds=%v err=%v", binds, err)
	}
}

func TestSandboxLaunchArgsFailClosedAndIsolated(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed in this local test environment")
	}
	workspace := t.TempDir()
	executable := "/usr/bin/true"
	if _, err := os.Stat(executable); err != nil {
		t.Skip("/usr/bin/true unavailable on test host")
	}
	options := sandboxOptions{Mode: "readonly", ContainerID: "wrapper-bwrap-test"}
	_, args, env, err := buildSandboxLaunch(options, executable, []string{"value with spaces"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	contains := func(values []string, sequence ...string) bool {
		for i := 0; i+len(sequence) <= len(values); i++ {
			if reflect.DeepEqual(values[i:i+len(sequence)], sequence) {
				return true
			}
		}
		return false
	}
	for _, sequence := range [][]string{
		{"--unshare-user"}, {"--unshare-pid"}, {"--unshare-net"},
		{"--new-session"}, {"--cap-drop", "ALL"}, {"--clearenv"},
		{"--tmpfs", "/tmp"}, {"--ro-bind", workspace, workspace},
		{"--chdir", workspace}, {"--setenv", "AGENT_EBPF_CONTAINER_ID", "wrapper-bwrap-test"},
		{"--", executable, "value with spaces"},
	} {
		if !contains(args, sequence...) {
			t.Fatalf("missing sandbox args %q in %q", sequence, args)
		}
	}
	if contains(args, "--ro-bind", "/", "/") {
		t.Fatal("sandbox must not expose the host root filesystem")
	}
	if !reflect.DeepEqual(env, []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}) {
		t.Fatalf("unsafe helper environment: %v", env)
	}

	options.Mode = "workspace"
	_, rwArgs, _, err := buildSandboxLaunch(options, executable, nil, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(rwArgs, "--bind", workspace, workspace) {
		t.Fatal("writable workspace flag did not bind workspace")
	}
	options.Network = true
	_, networkArgs, _, err := buildSandboxLaunch(options, executable, nil, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if contains(networkArgs, "--unshare-net") {
		t.Fatal("explicit network mode should share host network namespace")
	}
}

func TestSandboxLaunchRejectsOutsideWorkspace(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	workspace := t.TempDir()
	outside := t.TempDir()
	_, _, _, err := buildSandboxLaunch(sandboxOptions{Mode: "workspace", Workspace: workspace}, "/usr/bin/true", nil, outside)
	if err == nil || !strings.Contains(err.Error(), "outside sandbox workspace") {
		t.Fatalf("outside workspace error=%v", err)
	}

	bad := []sandboxOptions{
		{Mode: "readonly", Workspace: "/"},
		{Mode: "readonly", Workspace: "/usr"},
		{Mode: "readonly", ReadOnly: []string{"/"}},
		{Mode: "readonly", ReadOnly: []string{"relative"}},
	}
	for _, opts := range bad {
		_, _, _, err := buildSandboxLaunch(opts, "/usr/bin/true", nil, workspace)
		if err == nil {
			t.Fatalf("unsafe sandbox opts accepted: %+v", opts)
		}
	}
}

func TestSandboxLaunchNeedsExplicitNonSystemExecutableBind(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	workspace := t.TempDir()
	externalDir := t.TempDir()
	target := filepath.Join(externalDir, "tool")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	opts := sandboxOptions{Mode: "readonly"}
	_, _, _, err := buildSandboxLaunch(opts, target, nil, workspace)
	if err == nil || !strings.Contains(err.Error(), "--sandbox-ro-bind") {
		t.Fatalf("unmounted executable should be rejected: %v", err)
	}
	opts.ReadOnly = []string{externalDir}
	_, args, _, err := buildSandboxLaunch(opts, target, nil, workspace)
	if err != nil {
		t.Fatal(err)
	}
	needle := []string{"--ro-bind", externalDir, externalDir}
	if !strings.Contains(strings.Join(args, "\x00"), strings.Join(needle, "\x00")) {
		t.Fatalf("explicit bind missing: %v", args)
	}
}
