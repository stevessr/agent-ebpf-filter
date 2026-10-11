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
		status int
		want bool
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
	testCases := []struct{
		event *pb.Event
		want bool
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
