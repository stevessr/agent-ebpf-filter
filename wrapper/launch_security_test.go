package main

import (
	"errors"
	"os/user"
	"reflect"
	"strings"
	"testing"
)

type fakeLaunchIdentity struct {
	target       *user.User
	groupIDs     []string
	lookupErr    error
	groupIDsErr  error
	setgroupsErr error
	setgidErr    error
	setuidErr    error
	chdirErr     error
	euid         int
	egid         int
	groups       []int
	calls        []string
}

func (fake *fakeLaunchIdentity) ops() launchSecurityOps {
	return launchSecurityOps{
		chdir: func(path string) error {
			fake.calls = append(fake.calls, "chdir:"+path)
			return fake.chdirErr
		},
		lookupUser: func(name string) (*user.User, error) {
			fake.calls = append(fake.calls, "lookup:"+name)
			return fake.target, fake.lookupErr
		},
		groupIDs: func(*user.User) ([]string, error) {
			fake.calls = append(fake.calls, "groupids")
			return append([]string(nil), fake.groupIDs...), fake.groupIDsErr
		},
		geteuid:   func() int { return fake.euid },
		getegid:   func() int { return fake.egid },
		getgroups: func() ([]int, error) { return append([]int(nil), fake.groups...), nil },
		setgroups: func(groups []int) error {
			fake.calls = append(fake.calls, "setgroups")
			if fake.setgroupsErr == nil {
				fake.groups = append([]int(nil), groups...)
			}
			return fake.setgroupsErr
		},
		setgid: func(gid int) error {
			fake.calls = append(fake.calls, "setgid")
			if fake.setgidErr == nil {
				fake.egid = gid
			}
			return fake.setgidErr
		},
		setuid: func(uid int) error {
			fake.calls = append(fake.calls, "setuid")
			if fake.setuidErr == nil {
				fake.euid = uid
			}
			return fake.setuidErr
		},
	}
}

func newRootLaunchIdentity() *fakeLaunchIdentity {
	return &fakeLaunchIdentity{
		target:   &user.User{Username: "alice", Uid: "1001", Gid: "100"},
		groupIDs: []string{"300", "100", "200", "300"},
		euid:     0,
		egid:     0,
		groups:   []int{0},
	}
}

func TestSwitchUserWithOpsInitializesGroupsBeforeDroppingPrivileges(t *testing.T) {
	fake := newRootLaunchIdentity()
	if err := switchUserWithOps(" alice ", fake.ops()); err != nil {
		t.Fatalf("switchUserWithOps() error = %v", err)
	}
	wantCalls := []string{
		"lookup:alice",
		"groupids",
		"setgroups",
		"setgid",
		"setuid",
	}
	if !reflect.DeepEqual(fake.calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", fake.calls, wantCalls)
	}
	if fake.euid != 1001 || fake.egid != 100 {
		t.Fatalf("identity = uid:%d gid:%d", fake.euid, fake.egid)
	}
	if want := []int{100, 200, 300}; !reflect.DeepEqual(fake.groups, want) {
		t.Fatalf("groups = %v, want %v", fake.groups, want)
	}
}

func TestSwitchUserWithOpsFailsClosedAtEachTransition(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*fakeLaunchIdentity)
		wantError  string
		forbidCall string
	}{
		{
			name: "lookup",
			configure: func(fake *fakeLaunchIdentity) {
				fake.lookupErr = errors.New("missing user")
			},
			wantError:  "cannot resolve user",
			forbidCall: "groupids",
		},
		{
			name: "groups",
			configure: func(fake *fakeLaunchIdentity) {
				fake.groupIDsErr = errors.New("group database unavailable")
			},
			wantError:  "cannot resolve groups",
			forbidCall: "setgroups",
		},
		{
			name: "setgroups",
			configure: func(fake *fakeLaunchIdentity) {
				fake.setgroupsErr = errors.New("operation not permitted")
			},
			wantError:  "cannot set supplementary groups",
			forbidCall: "setgid",
		},
		{
			name: "setgid",
			configure: func(fake *fakeLaunchIdentity) {
				fake.setgidErr = errors.New("operation not permitted")
			},
			wantError:  "cannot setgid",
			forbidCall: "setuid",
		},
		{
			name: "setuid",
			configure: func(fake *fakeLaunchIdentity) {
				fake.setuidErr = errors.New("operation not permitted")
			},
			wantError: "cannot setuid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newRootLaunchIdentity()
			tt.configure(fake)
			err := switchUserWithOps("alice", fake.ops())
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantError)
			}
			if tt.forbidCall != "" && containsString(fake.calls, tt.forbidCall) {
				t.Fatalf("calls = %v, must not include %q", fake.calls, tt.forbidCall)
			}
		})
	}
}

func TestSwitchUserWithOpsAcceptsVerifiedExistingIdentity(t *testing.T) {
	fake := &fakeLaunchIdentity{
		target:   &user.User{Username: "alice", Uid: "1001", Gid: "100"},
		groupIDs: []string{"100", "200", "300"},
		euid:     1001,
		egid:     100,
		// Simulate platforms where getgroups omits the primary effective GID.
		groups: []int{300, 200},
	}
	if err := switchUserWithOps("alice", fake.ops()); err != nil {
		t.Fatalf("switchUserWithOps() error = %v", err)
	}
	for _, forbidden := range []string{"setgroups", "setgid", "setuid"} {
		if containsString(fake.calls, forbidden) {
			t.Fatalf("calls = %v, must not include %q", fake.calls, forbidden)
		}
	}
}

func TestSwitchUserWithOpsRejectsUnfixableExistingIdentity(t *testing.T) {
	fake := &fakeLaunchIdentity{
		target:   &user.User{Username: "alice", Uid: "1001", Gid: "100"},
		groupIDs: []string{"100", "200"},
		euid:     1001,
		egid:     100,
		groups:   []int{200, 999},
	}
	err := switchUserWithOps("alice", fake.ops())
	if err == nil || !strings.Contains(err.Error(), "supplementary groups") {
		t.Fatalf("error = %v, want supplementary group mismatch", err)
	}
}

func TestSwitchUserWithOpsRejectsInvalidNumericIdentity(t *testing.T) {
	fake := newRootLaunchIdentity()
	fake.target.Uid = "not-a-number"
	err := switchUserWithOps("alice", fake.ops())
	if err == nil || !strings.Contains(err.Error(), "invalid user") {
		t.Fatalf("error = %v, want invalid identity", err)
	}
	if containsString(fake.calls, "setgroups") {
		t.Fatalf("calls = %v, privilege transition must not start", fake.calls)
	}
}

func TestPrepareAndExecuteDoesNotExecuteAfterPreparationFailure(t *testing.T) {
	tests := []struct {
		name      string
		cwd       string
		username  string
		configure func(*fakeLaunchIdentity)
	}{
		{
			name:     "invalid user",
			username: "missing",
			configure: func(fake *fakeLaunchIdentity) {
				fake.lookupErr = errors.New("missing user")
			},
		},
		{
			name: "invalid cwd",
			cwd:  "/missing",
			configure: func(fake *fakeLaunchIdentity) {
				fake.euid = 1001
				fake.egid = 100
				fake.groups = []int{100, 200, 300}
				fake.chdirErr = errors.New("no such directory")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newRootLaunchIdentity()
			tt.configure(fake)
			executed := false
			err := prepareAndExecute(tt.cwd, tt.username, fake.ops(), func() {
				executed = true
			})
			if err == nil {
				t.Fatal("prepareAndExecute() error = nil")
			}
			if executed {
				t.Fatal("command executed after launch preparation failure")
			}
		})
	}
}

func TestPrepareAndExecuteRequiresExplicitUserWhenRunningAsRoot(t *testing.T) {
	fake := newRootLaunchIdentity()
	executed := false
	err := prepareAndExecute("", "", fake.ops(), func() {
		executed = true
	})
	if err == nil || !strings.Contains(err.Error(), "explicit --user") {
		t.Fatalf("error = %v, want explicit user requirement", err)
	}
	if executed {
		t.Fatal("command executed as implicit root")
	}
}

func TestPrepareAndExecuteAllowsImplicitCurrentNonRootUser(t *testing.T) {
	fake := newRootLaunchIdentity()
	fake.euid = 1001
	fake.egid = 100
	executed := false
	if err := prepareAndExecute("", "", fake.ops(), func() {
		executed = true
	}); err != nil {
		t.Fatalf("prepareAndExecute() error = %v", err)
	}
	if !executed {
		t.Fatal("command was not executed for non-root caller")
	}
}

func TestPrepareAndExecuteRunsAfterVerifiedPreparation(t *testing.T) {
	fake := newRootLaunchIdentity()
	executed := false
	if err := prepareAndExecute("/work", "alice", fake.ops(), func() {
		executed = true
	}); err != nil {
		t.Fatalf("prepareAndExecute() error = %v", err)
	}
	if !executed {
		t.Fatal("command was not executed")
	}
	if got := fake.calls[len(fake.calls)-1]; got != "chdir:/work" {
		t.Fatalf("last preparation call = %q, want chdir", got)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
