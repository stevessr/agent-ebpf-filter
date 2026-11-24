package app

import (
	"agent-ebpf-filter/app/platform"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func ensureBackendPrivileges() (bool, error) {
	if os.Geteuid() == 0 {
		return false, nil
	}

	realHome, _ := os.UserHomeDir()

	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	priv, err := privilegeEscalationCmd()
	if err != nil {
		return false, err
	}
	cmd := privilegedCommand(priv, exe, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = os.Environ()

	if realHome != "" {
		cmd.Env = platform.SetEnvValue(cmd.Env, "AGENT_REAL_HOME", realHome)
	}

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("start privileged backend: %w", err)
	}
	return true, nil
}

func ensurePinnedMapPermissions() error {
	for _, dir := range []string{ebpfPinRoot, ebpfPinMapsDir, ebpfPinLinksDir} {
		if err := os.Chmod(dir, 0755); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("chmod %s: %w", dir, err)
		}
	}
	for _, name := range mapNames {
		path := filepath.Join(ebpfPinMapsDir, name)
		if err := os.Chmod(path, 0600); err != nil {
			return fmt.Errorf("chmod map %s: %w", name, err)
		}
	}
	return nil
}

func privilegedCommand(priv, exe string, args ...string) *exec.Cmd {
	if filepath.Base(priv) == "sudo" {
		sudoArgs := []string{"--preserve-env=AGENT_WRAPPER_PATH,DISPLAY,WAYLAND_DISPLAY,XAUTHORITY,USER,HOME,AGENT_REAL_HOME,GIN_MODE,DISABLE_AUTH,AGENT_BACKEND_HOST,AGENT_BACKEND_PORT,AGENT_WS_ALLOWED_ORIGINS", exe}
		if os.Getenv("SUDO_ASKPASS") != "" {
			sudoArgs = append([]string{"-A"}, sudoArgs...)
		}
		sudoArgs = append(sudoArgs, args...)
		return exec.Command(priv, sudoArgs...)
	}

	cmdArgs := append([]string{exe}, args...)
	if filepath.Base(priv) == "pkexec" && os.Getenv("AGENT_DESKTOP_LIFETIME_SOCKET") != "" {
		cmdArgs = append([]string{"--disable-internal-agent"}, cmdArgs...)
	}
	cmd := exec.Command(priv, cmdArgs...)

	// Manual environment inheritance for non-sudo escalators (like pkexec)
	// We want to pass down only selected safe/required variables.
	whitelist := map[string]bool{
		"AGENT_BACKEND_HOST":       true,
		"AGENT_BACKEND_PORT":       true,
		"AGENT_WS_ALLOWED_ORIGINS": true,
		"AGENT_WRAPPER_PATH":       true,
		"AGENT_REAL_HOME":          true,
		"GIN_MODE":                 true,
		"DISABLE_AUTH":             true,
		"DISPLAY":                  true,
		"WAYLAND_DISPLAY":          true,
		"XAUTHORITY":               true,
	}

	inherited := os.Environ()
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 && whitelist[parts[0]] {
			inherited = platform.SetEnvValue(inherited, parts[0], parts[1])
		}
	}
	cmd.Env = inherited

	return cmd
}

func privilegeEscalationCmd() (string, error) {
	if os.Getenv("SUDO_ASKPASS") != "" {
		if p, err := exec.LookPath("sudo"); err == nil {
			return p, nil
		}
	}
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		if p, err := exec.LookPath("pkexec"); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("sudo"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("pkexec"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("need sudo or pkexec for privileged backend startup")
}
