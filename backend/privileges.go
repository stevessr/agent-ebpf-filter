package main

import (
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

func originalInvokerIDs() (uid, gid uint32, ok bool) {
	if uidStr := os.Getenv("SUDO_UID"); uidStr != "" {
		gidStr := os.Getenv("SUDO_GID")
		if gidStr == "" {
			return 0, 0, false
		}
		parsedUID, err1 := strconv.ParseUint(uidStr, 10, 32)
		parsedGID, err2 := strconv.ParseUint(gidStr, 10, 32)
		if err1 != nil || err2 != nil {
			return 0, 0, false
		}
		return uint32(parsedUID), uint32(parsedGID), true
	}

	if uidStr := os.Getenv("PKEXEC_UID"); uidStr != "" {
		u, err := user.LookupId(uidStr)
		if err != nil {
			return 0, 0, false
		}
		parsedUID, err1 := strconv.ParseUint(uidStr, 10, 32)
		parsedGID, err2 := strconv.ParseUint(u.Gid, 10, 32)
		if err1 != nil || err2 != nil {
			return 0, 0, false
		}
		return uint32(parsedUID), uint32(parsedGID), true
	}

	return 0, 0, false
}

func allowedControlPlaneUIDs() map[uint32]struct{} {
	allowed := map[uint32]struct{}{
		uint32(os.Getuid()): {},
	}
	if uid, _, ok := originalInvokerIDs(); ok {
		allowed[uid] = struct{}{}
	}
	return allowed
}

func applyCredentialToCommand(cmd *exec.Cmd, uid, gid uint32, uidStr string) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uid, Gid: gid}

	if u, err := user.LookupId(uidStr); err == nil {
		if cmd.Env == nil {
			cmd.Env = os.Environ()
		}
		cmd.Env = setEnvValue(cmd.Env, "USER", u.Username)
		cmd.Env = setEnvValue(cmd.Env, "LOGNAME", u.Username)
		cmd.Env = setEnvValue(cmd.Env, "HOME", u.HomeDir)
	}
}

func configureCommandForRealUser(cmd *exec.Cmd) {
	if os.Getuid() != 0 {
		return
	}

	if uid, gid, ok := originalInvokerIDs(); ok {
		applyCredentialToCommand(cmd, uid, gid, strconv.FormatUint(uint64(uid), 10))
	}
}

// dropPrivileges modifies cmd.SysProcAttr to run the command as the original
// invoking user (SUDO_UID/SUDO_GID) instead of root, mitigating security risks
// when executing shells or arbitrary commands from the backend.
// It also updates the HOME and USER environment variables.
func dropPrivileges(cmd *exec.Cmd) {
	configureCommandForRealUser(cmd)
}
