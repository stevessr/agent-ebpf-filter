package main

import (
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

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

	if uidStr := os.Getenv("SUDO_UID"); uidStr != "" {
		gidStr := os.Getenv("SUDO_GID")
		if gidStr == "" {
			return
		}
		uid, err1 := strconv.ParseUint(uidStr, 10, 32)
		gid, err2 := strconv.ParseUint(gidStr, 10, 32)
		if err1 != nil || err2 != nil {
			return
		}
		applyCredentialToCommand(cmd, uint32(uid), uint32(gid), uidStr)
		return
	}

	if uidStr := os.Getenv("PKEXEC_UID"); uidStr != "" {
		u, err := user.LookupId(uidStr)
		if err != nil {
			return
		}
		uid, err1 := strconv.ParseUint(uidStr, 10, 32)
		gid, err2 := strconv.ParseUint(u.Gid, 10, 32)
		if err1 != nil || err2 != nil {
			return
		}
		applyCredentialToCommand(cmd, uint32(uid), uint32(gid), uidStr)
	}
}

// dropPrivileges modifies cmd.SysProcAttr to run the command as the original
// invoking user (SUDO_UID/SUDO_GID) instead of root, mitigating security risks
// when executing shells or arbitrary commands from the backend.
// It also updates the HOME and USER environment variables.
func dropPrivileges(cmd *exec.Cmd) {
	configureCommandForRealUser(cmd)
}
