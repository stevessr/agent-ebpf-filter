package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func configureBackendLauncher(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// configureBackendAskpass lets the Renew executable itself satisfy sudo's
// askpass protocol. The internal backend child sees --internal-backend first,
// while sudo later invokes the same executable without that flag and with
// AGENT_RENEW_ASKPASS=1, so main dispatches to runAskpass.
func configureBackendAskpass(cmd *exec.Cmd, binary string) {
	if cmd == nil || strings.TrimSpace(binary) == "" || os.Getenv("SUDO_ASKPASS") != "" {
		return
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return
	}
	hasDialog := false
	for _, name := range []string{"zenity", "kdialog"} {
		if _, err := exec.LookPath(name); err == nil {
			hasDialog = true
			break
		}
	}
	if !hasDialog {
		return
	}
	cmd.Env = append(cmd.Env,
		"SUDO_ASKPASS="+binary,
		askpassModeEnv+"=1",
	)
}

func isAskpassInvocation() bool {
	if os.Getenv(askpassModeEnv) == "1" {
		return true
	}
	if os.Geteuid() == 0 {
		return false
	}
	parent, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", os.Getppid()))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(parent)) == "sudo"
}

func runAskpass() error {
	prompt := "请输入登录密码以启动 eBPF 后端："
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[len(os.Args)-1]) != "" {
		prompt = strings.TrimSpace(os.Args[len(os.Args)-1])
	}

	if binary, err := exec.LookPath("zenity"); err == nil {
		cmd := exec.Command(binary, "--password", "--title=Renew 后端授权", "--text="+prompt)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	if binary, err := exec.LookPath("kdialog"); err == nil {
		cmd := exec.Command(binary, "--title", "Renew 后端授权", "--password", prompt)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("Renew needs a PolicyKit authentication agent, a configured SUDO_ASKPASS, or zenity/kdialog")
}

func cancelBackendLauncher(cmd *exec.Cmd) {
	// Cancel the owned launcher and any authorization helper still running as
	// the calling user. Connected root backends are stopped by socket EOF.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}
