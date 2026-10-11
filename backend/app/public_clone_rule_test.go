package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"agent-ebpf-filter/pb"
)

func TestPublicGitHubCloneRepositoryIsStrict(t *testing.T) {
	tests := []struct {
		name, comm string
		args       []string
		wantRepo   string
	}{
		{"https git clone", "git", []string{"clone", "https://github.com/example/project.git"}, "example/project"},
		{"gh repo clone", "gh", []string{"repo", "clone", "example/project"}, "example/project"},
		{"git shallow clone", "git", []string{"clone", "--depth", "1", "https://github.com/example/project.git"}, "example/project"},
		{"git branch clone", "git", []string{"clone", "-b", "release/1.0", "--single-branch", "https://github.com/example/project"}, "example/project"},
		{"git filtered clone", "git", []string{"clone", "--filter=blob:none", "https://github.com/example/project"}, "example/project"},
		{"gh shallow clone", "gh", []string{"repo", "clone", "example/project", "--", "--depth", "1"}, "example/project"},
		{"gh clone directory and flags", "gh", []string{"repo", "clone", "example/project", "local", "--", "--filter=blob:none"}, "example/project"},
		{"gh repo HTTPS", "gh", []string{"repo", "clone", "https://github.com/example/project.git"}, "example/project"},
		{"git hook injection", "git", []string{"clone", "-c", "core.hooksPath=/tmp/hooks", "https://github.com/example/project"}, ""},
		{"git upload-pack injection", "git", []string{"clone", "--upload-pack=sh", "https://github.com/example/project"}, ""},
		{"git recurse submodules", "git", []string{"clone", "--recurse-submodules", "https://github.com/example/project"}, ""},
		{"git alternate transport", "git", []string{"clone", "--config=protocol.file.allow=always", "https://github.com/example/project"}, ""},
		{"git invalid depth", "git", []string{"clone", "--depth", "0", "https://github.com/example/project"}, ""},
		{"git oversized depth", "git", []string{"clone", "--depth", "99999999", "https://github.com/example/project"}, ""},
		{"gh unsafe git option", "gh", []string{"repo", "clone", "example/project", "--", "--upload-pack=sh"}, ""},
		{"gh unsafe git operand", "gh", []string{"repo", "clone", "example/project", "--", "https://evil.example/repo"}, ""},
		{"gh double dash with no options", "gh", []string{"repo", "clone", "example/project", "--"}, ""},
		{"encoded path", "git", []string{"clone", "https://github.com/example%2fother/project"}, ""},

		{"git destination", "git", []string{"clone", "https://github.com/example/project", "src"}, "example/project"},
		{"private-capable ssh", "git", []string{"clone", "git@github.com:example/project.git"}, ""},
		{"credentialed https", "git", []string{"clone", "https://u:token@github.com/example/project.git"}, ""},
		{"lookalike domain", "git", []string{"clone", "https://github.com.evil.invalid/example/project"}, ""},
		{"non-clone command", "gh", []string{"repo", "delete", "example/project"}, ""},
		{"git push", "git", []string{"push", "https://github.com/example/project"}, ""},
		{"option inject", "git", []string{"clone", "--upload-pack=sh", "https://github.com/example/project"}, ""},
		{"url query", "git", []string{"clone", "https://github.com/example/project?token=abc"}, ""},
		{"path traversal", "git", []string{"clone", "https://github.com/example/project", "../../elsewhere"}, ""},
		{"gh bad destination", "gh", []string{"repo", "clone", "example/project", "/tmp/outside"}, ""},
		{"extra command", "git", []string{"clone", "https://github.com/example/project", "dest", "--config=foo"}, ""},
		{"different host", "git", []string{"clone", "https://gitlab.com/example/project"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, ok := publicGitHubCloneRepository(tt.comm, tt.args)
			if ok != (tt.wantRepo != "") || repo != tt.wantRepo {
				t.Fatalf("publicGitHubCloneRepository(%q, %v) = (%q, %v), want %q", tt.comm, tt.args, repo, ok, tt.wantRepo)
			}
		})
	}
}

func TestPublicGitHubRepositoryRequiresAnonymousPublicMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		status         int
		want           bool
	}{
		{"public", `{"full_name":"example/project","private":false,"visibility":"public"}`, http.StatusOK, true},
		{"private", `{"full_name":"example/project","private":true,"visibility":"private"}`, http.StatusOK, false},
		{"incomplete", `{"full_name":"example/project"}`, http.StatusOK, false},
		{"different repo", `{"full_name":"different/project","private":false}`, http.StatusOK, false},
		{"not found", `{"message":"Not Found"}`, http.StatusNotFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/example/project" {
					t.Errorf("unexpected request path %s", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.response)
			}))
			defer server.Close()
			if got := checkPublicGitHubRepository(context.Background(), server.Client(), server.URL, "example/project"); got != tc.want {
				t.Fatalf("visibility verified = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVerifiedPublicCloneExemptsOnlyCloneProcessLoopHeuristics(t *testing.T) {
	const pid uint32 = 42731
	markPublicCloneProcess(pid, "gh")
	t.Cleanup(func() { unmarkPublicCloneProcess(pid) })
	testCases := []struct {
		event *pb.Event
		want  bool
	}{
		{&pb.Event{Pid: pid, Comm: "gh", Type: "clone"}, true},
		{&pb.Event{Pid: pid + 1, Ppid: pid, Comm: "git", Type: "openat"}, true},
		{&pb.Event{Pid: pid + 2, Ppid: pid, Comm: "curl", Type: "network_connect"}, false},
		{&pb.Event{Pid: pid, Comm: "bash", Type: "process_exec"}, false},
		{&pb.Event{Pid: pid + 3, Comm: "gh", Type: "clone"}, false},
	}
	for _, tc := range testCases {
		if got := isVerifiedPublicCloneProcess(tc.event); got != tc.want {
			t.Fatalf("trusted clone for %+v = %v, want %v", tc.event, got, tc.want)
		}
	}
}

func TestVerifiedPublicCloneOnlyPropagatesThroughGitTransportStarts(t *testing.T) {
	const root uint32 = 52800
	markPublicCloneProcess(root, "gh")
	t.Cleanup(func() {
		for _, pid := range []uint32{root, root + 1, root + 2, root + 3} {
			unmarkPublicCloneProcess(pid)
		}
	})
	for _, event := range []*pb.Event{
		{Pid: root + 1, Ppid: root, Comm: "git", Type: "process_exec"},
		{Pid: root + 2, Ppid: root + 1, Comm: "git-remote-https", Type: "process_exec"},
		{Pid: root + 3, Ppid: root + 2, Comm: "git-index-pack", Type: "openat"},
	} {
		if !isVerifiedPublicCloneProcess(event) {
			t.Fatalf("verified Git clone transport child should inherit loop threshold: %+v", event)
		}
	}
	for _, event := range []*pb.Event{
		{Pid: root + 4, Ppid: root, Comm: "bash", Type: "process_exec"},
		{Pid: root + 5, Ppid: root + 1, Comm: "curl", Type: "process_exec"},
		{Pid: root + 6, Ppid: root + 3, Comm: "git-index-pack", Type: "openat"},
	} {
		if isVerifiedPublicCloneProcess(event) {
			t.Fatalf("unrelated or unverified deep descendant inherited exemption: %+v", event)
		}
	}
	if isVerifiedPublicCloneProcess(&pb.Event{Pid: root + 2, Comm: "git-remote-https", Type: "process_exit"}) {
		t.Fatal("exited Git transport PID must lose its exemption")
	}
	if isVerifiedPublicCloneProcess(&pb.Event{Pid: root + 7, Ppid: root + 2, Comm: "git-index-pack", Type: "openat"}) {
		t.Fatal("descendants of a terminated process must not inherit trust")
	}
}
