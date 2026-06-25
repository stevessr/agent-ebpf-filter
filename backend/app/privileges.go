package app

import (
	"agent-ebpf-filter/app/platform"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// ---- moved from backend/zz_merged_backend.go section privileges.go ----


func allowedControlPlaneUIDs() map[uint32]struct{} {
	allowed := map[uint32]struct{}{
		uint32(os.Getuid()): {},
	}
	if uid, _, ok := platform.OriginalInvokerIDs(); ok {
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
		cmd.Env = platform.SetEnvValue(cmd.Env, "USER", u.Username)
		cmd.Env = platform.SetEnvValue(cmd.Env, "LOGNAME", u.Username)
		cmd.Env = platform.SetEnvValue(cmd.Env, "HOME", u.HomeDir)
	}
}

func configureCommandForRealUser(cmd *exec.Cmd) {
	if os.Getuid() != 0 {
		return
	}

	if uid, gid, ok := platform.OriginalInvokerIDs(); ok {
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
