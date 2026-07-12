package main

import (
	"fmt"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type launchSecurityOps struct {
	chdir      func(string) error
	lookupUser func(string) (*user.User, error)
	groupIDs   func(*user.User) ([]string, error)
	geteuid    func() int
	getegid    func() int
	getgroups  func() ([]int, error)
	setgroups  func([]int) error
	setgid     func(int) error
	setuid     func(int) error
}

func defaultLaunchSecurityOps() launchSecurityOps {
	return launchSecurityOps{
		chdir:      os.Chdir,
		lookupUser: user.Lookup,
		groupIDs:   func(target *user.User) ([]string, error) { return target.GroupIds() },
		geteuid:    os.Geteuid,
		getegid:    os.Getegid,
		getgroups:  os.Getgroups,
		setgroups:  syscall.Setgroups,
		setgid:     syscall.Setgid,
		setuid:     syscall.Setuid,
	}
}

func prepareAndExecute(
	cwd string,
	username string,
	ops launchSecurityOps,
	executeCommand func(),
) error {
	username = strings.TrimSpace(username)
	if username != "" {
		if err := switchUserWithOps(username, ops); err != nil {
			return err
		}
	} else if ops.geteuid() == 0 {
		return fmt.Errorf("refusing to execute as root without an explicit --user")
	}
	if strings.TrimSpace(cwd) != "" {
		if err := ops.chdir(cwd); err != nil {
			return fmt.Errorf("cannot change working directory to %q: %w", cwd, err)
		}
	}
	if executeCommand == nil {
		return fmt.Errorf("command execution is unavailable")
	}
	executeCommand()
	return nil
}

func switchUser(username string) error {
	return switchUserWithOps(username, defaultLaunchSecurityOps())
}

func switchUserWithOps(username string, ops launchSecurityOps) error {
	username = strings.TrimSpace(username)
	target, err := ops.lookupUser(username)
	if err != nil {
		return fmt.Errorf("cannot resolve user %q: %w", username, err)
	}

	uid, err := parseIdentityID("uid", target.Uid)
	if err != nil {
		return fmt.Errorf("invalid user %q: %w", username, err)
	}
	gid, err := parseIdentityID("gid", target.Gid)
	if err != nil {
		return fmt.Errorf("invalid user %q: %w", username, err)
	}
	groups, err := targetGroupIDs(target, gid, ops.groupIDs)
	if err != nil {
		return fmt.Errorf("cannot resolve groups for user %q: %w", username, err)
	}

	if ops.geteuid() != 0 {
		return verifyExistingIdentity(username, uid, gid, groups, ops)
	}

	// Supplementary groups must be installed before dropping GID/UID.
	if err := ops.setgroups(groups); err != nil {
		return fmt.Errorf("cannot set supplementary groups for user %q: %w", username, err)
	}
	if err := ops.setgid(gid); err != nil {
		return fmt.Errorf("cannot setgid to %d for user %q: %w", gid, username, err)
	}
	if err := ops.setuid(uid); err != nil {
		return fmt.Errorf("cannot setuid to %d for user %q: %w", uid, username, err)
	}
	return verifyExistingIdentity(username, uid, gid, groups, ops)
}

func parseIdentityID(kind, raw string) (int, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s %q is not valid: %w", kind, raw, err)
	}
	return int(value), nil
}

func targetGroupIDs(
	target *user.User,
	primaryGID int,
	lookup func(*user.User) ([]string, error),
) ([]int, error) {
	rawGroups, err := lookup(target)
	if err != nil {
		return nil, err
	}
	seen := map[int]struct{}{primaryGID: {}}
	for _, raw := range rawGroups {
		gid, err := parseIdentityID("group id", raw)
		if err != nil {
			return nil, err
		}
		seen[gid] = struct{}{}
	}
	groups := make([]int, 0, len(seen))
	for gid := range seen {
		groups = append(groups, gid)
	}
	sort.Ints(groups)
	return groups, nil
}

func verifyExistingIdentity(
	username string,
	uid int,
	gid int,
	wantGroups []int,
	ops launchSecurityOps,
) error {
	if got := ops.geteuid(); got != uid {
		return fmt.Errorf("refusing to execute as user %q: effective uid is %d, want %d", username, got, uid)
	}
	if got := ops.getegid(); got != gid {
		return fmt.Errorf("refusing to execute as user %q: effective gid is %d, want %d", username, got, gid)
	}
	gotGroups, err := ops.getgroups()
	if err != nil {
		return fmt.Errorf("cannot verify supplementary groups for user %q: %w", username, err)
	}
	// getgroups(2) may omit the effective primary GID; include it explicitly
	// before comparing the complete target identity.
	gotGroups = append(gotGroups, gid)
	if !sameIdentityGroups(gotGroups, wantGroups) {
		return fmt.Errorf(
			"refusing to execute as user %q: supplementary groups are %v, want %v",
			username,
			normalizeIdentityGroups(gotGroups),
			normalizeIdentityGroups(wantGroups),
		)
	}
	return nil
}

func sameIdentityGroups(left, right []int) bool {
	left = normalizeIdentityGroups(left)
	right = normalizeIdentityGroups(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func normalizeIdentityGroups(groups []int) []int {
	seen := make(map[int]struct{}, len(groups))
	for _, gid := range groups {
		seen[gid] = struct{}{}
	}
	normalized := make([]int, 0, len(seen))
	for gid := range seen {
		normalized = append(normalized, gid)
	}
	sort.Ints(normalized)
	return normalized
}
