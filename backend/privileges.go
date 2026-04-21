package main

import (
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// dropPrivileges modifies cmd.SysProcAttr to run the command as the original
// invoking user (SUDO_UID/SUDO_GID) instead of root, mitigating security risks
// when executing shells or arbitrary commands from the backend.
// It also updates the HOME and USER environment variables.
func dropPrivileges(cmd *exec.Cmd) {
	if os.Getuid() != 0 {
		return
	}
	uidStr := os.Getenv("SUDO_UID")
	gidStr := os.Getenv("SUDO_GID")
	if uidStr == "" || gidStr == "" {
		return
	}
	uid, err1 := strconv.ParseUint(uidStr, 10, 32)
	gid, err2 := strconv.ParseUint(gidStr, 10, 32)
	if err1 != nil || err2 != nil {
		return
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}

	// Lookup user to set correct environment variables
	if u, err := user.LookupId(uidStr); err == nil {
		if cmd.Env == nil {
			cmd.Env = os.Environ()
		}
		cmd.Env = setEnvValue(cmd.Env, "USER", u.Username)
		cmd.Env = setEnvValue(cmd.Env, "LOGNAME", u.Username)
		cmd.Env = setEnvValue(cmd.Env, "HOME", u.HomeDir)
	}
}
