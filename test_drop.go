package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

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
}

func main() {
	cmd := exec.Command("id")
	dropPrivileges(cmd)
	out, err := cmd.CombinedOutput()
	fmt.Printf("id: %s (err: %v)\n", string(out), err)
}
