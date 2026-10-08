package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// sandboxOptions only affects launches that explicitly opt in. This launcher
// provides mount/namespace isolation; it is not a substitute for seccomp,
// gVisor's Sentry, or host-side eBPF/LSM enforcement.
type sandboxOptions struct {
	Mode       string
	Workspace  string
	Network    bool
	ReadOnly   []string
	ContainerID string
}

type sandboxBinds []string

func (binds *sandboxBinds) String() string { return strings.Join(*binds, ",") }
func (binds *sandboxBinds) Set(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("sandbox read-only bind cannot be empty")
	}
	*binds = append(*binds, path)
	return nil
}

func (s sandboxOptions) validate() error {
	switch s.Mode {
	case "off":
		if s.Workspace != "" || s.Network || len(s.ReadOnly) > 0 {
			return fmt.Errorf("sandbox options require --sandbox=readonly or --sandbox=workspace")
		}
	case "readonly", "workspace":
		if runtime.GOOS != "linux" {
			return fmt.Errorf("bubblewrap sandbox is only supported on Linux")
		}
	default:
		return fmt.Errorf("unsupported --sandbox mode %q (use off, readonly, workspace)", s.Mode)
	}
	return nil
}

func newSandboxContainerID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("create sandbox correlation id: %w", err)
	}
	return fmt.Sprintf("wrapper-bwrap-%x", id), nil
}

// buildSandboxLaunch checks every mount before creating the bwrap argv.
// No partial or best-effort sandbox fallback is permitted.
func buildSandboxLaunch(opts sandboxOptions, name string, commandArgs []string, cwd string) (string, []string, []string, error) {
	if err := opts.validate(); err != nil {
		return "", nil, nil, err
	}
	if opts.Mode == "off" {
		return "", nil, nil, fmt.Errorf("sandbox launch requested with sandbox disabled")
	}

	bwrapPath, err := exec.LookPath("bwrap")
	if err != nil {
		return "", nil, nil, fmt.Errorf("sandbox requested but bubblewrap (bwrap) is unavailable: %w", err)
	}
	bwrapPath, err = filepath.EvalSymlinks(bwrapPath)
	if err != nil {
		return "", nil, nil, fmt.Errorf("resolve bubblewrap: %w", err)
	}
	if !within(bwrapPath, "/usr") && !within(bwrapPath, "/bin") {
		return "", nil, nil, fmt.Errorf("refusing untrusted bubblewrap executable outside /usr or /bin: %q", bwrapPath)
	}

	workingDir, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", nil, nil, fmt.Errorf("resolve current working directory: %w", err)
	}
	workspace := opts.Workspace
	if workspace == "" {
		workspace = workingDir
	}
	if !filepath.IsAbs(workspace) {
		return "", nil, nil, fmt.Errorf("sandbox workspace must be an absolute path: %q", workspace)
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", nil, nil, fmt.Errorf("resolve sandbox workspace: %w", err)
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return "", nil, nil, fmt.Errorf("sandbox workspace must be an existing directory: %q", workspace)
	}
	for _, unsafe := range []string{"/", "/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc", "/dev", "/proc", "/sys", "/run", "/var", "/home", "/opt", "/tmp"} {
		if workspace == unsafe {
			return "", nil, nil, fmt.Errorf("sandbox workspace %q is too broad", workspace)
		}
	}
	if !within(workingDir, workspace) {
		return "", nil, nil, fmt.Errorf("working directory %q is outside sandbox workspace %q", workingDir, workspace)
	}
	executable, err := exec.LookPath(name)
	if err != nil {
		return "", nil, nil, fmt.Errorf("sandbox executable %q unavailable: %w", name, err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", nil, nil, fmt.Errorf("resolve sandbox executable: %w", err)
	}
	exeInfo, err := os.Stat(executable)
	if err != nil || exeInfo.IsDir() {
		return "", nil, nil, fmt.Errorf("sandbox command is not an executable file: %q", executable)
	}

	extra := make([]string, 0, len(opts.ReadOnly))
	for _, source := range opts.ReadOnly {
		if !filepath.IsAbs(source) {
			return "", nil, nil, fmt.Errorf("read-only bind path must be absolute: %q", source)
		}
		real, err := filepath.EvalSymlinks(source)
		if err != nil {
			return "", nil, nil, fmt.Errorf("resolve read-only bind %q: %w", source, err)
		}
		if real == "/" {
			return "", nil, nil, fmt.Errorf("refusing to expose the entire host filesystem with --sandbox-ro-bind")
		}
		if _, err := os.Stat(real); err != nil {
			return "", nil, nil, fmt.Errorf("read-only bind %q: %w", real, err)
		}
		extra = append(extra, real)
	}
	permitted := within(executable, "/usr") || within(executable, "/bin") ||
		within(executable, "/sbin") || within(executable, "/lib") ||
		within(executable, "/lib64") || within(executable, workspace)
	for _, root := range extra {
		permitted = permitted || within(executable, root)
	}
	if !permitted {
		return "", nil, nil, fmt.Errorf("executable %q is outside mounted paths; pass --sandbox-ro-bind for its directory", executable)
	}

	// Require an explicit user namespace. Disallow silently falling back to
	// weaker isolation when the kernel has unprivileged userns disabled.
	args := []string{
		"--unshare-user", "--unshare-ipc", "--unshare-pid",
		"--unshare-uts", "--unshare-cgroup-try",
		"--die-with-parent", "--new-session", "--cap-drop", "ALL",
		"--clearenv", "--setenv", "PATH", "/usr/local/bin:/usr/bin:/bin",
		"--setenv", "HOME", "/tmp", "--setenv", "TMPDIR", "/tmp",
		"--setenv", "XDG_CACHE_HOME", "/tmp/.cache",
		"--setenv", "XDG_CONFIG_HOME", "/tmp/.config",
		"--setenv", "LANG", "C.UTF-8",
		"--setenv", "AGENT_EBPF_SANDBOX_RUNTIME", "bubblewrap",
	}
	if !opts.Network {
		args = append(args, "--unshare-net")
	}
	if opts.ContainerID != "" {
		if strings.ContainsAny(opts.ContainerID, "\x00\r\n") || len(opts.ContainerID) > 128 {
			return "", nil, nil, fmt.Errorf("invalid sandbox container correlation id")
		}
		args = append(args, "--setenv", "AGENT_EBPF_CONTAINER_ID", opts.ContainerID)
	}
	// Establish an empty root, not a readonly bind of host /. In particular,
	// secrets under /root, /home, /etc and /run are not automatically exposed.
	args = append(args,
		"--dir", "/usr", "--dir", "/etc", "--dir", "/home",
		"--dir", "/run", "--dir", "/var", "--dir", "/opt",
		"--dir", "/tmp", "--dir", "/mnt", "--dir", "/media",
		"--proc", "/proc", "--dev", "/dev",
	)
	if _, err := os.Stat("/usr"); err != nil {
		return "", nil, nil, fmt.Errorf("sandbox runtime /usr not available: %w", err)
	}
	args = append(args, "--ro-bind", "/usr", "/usr")
	for _, root := range []string{"/bin", "/sbin", "/lib", "/lib64"} {
		stat, err := os.Lstat(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", nil, nil, fmt.Errorf("inspect system runtime mount %q: %w", root, err)
		}
		if stat.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(root)
			if err != nil {
				return "", nil, nil, err
			}
			args = append(args, "--symlink", target, root)
		} else {
			args = append(args, "--ro-bind", root, root)
		}
	}
	// Narrow identity/config essentials. No full /etc, host home or credentials.
	for _, file := range []string{"/etc/passwd", "/etc/group", "/etc/nsswitch.conf", "/etc/hosts"} {
		if fi, err := os.Stat(file); err == nil && fi.Mode().IsRegular() {
			args = append(args, "--ro-bind", file, file)
		}
	}
	if opts.Network {
		if fi, err := os.Stat("/etc/resolv.conf"); err == nil && fi.Mode().IsRegular() {
			args = append(args, "--ro-bind", "/etc/resolv.conf", "/etc/resolv.conf")
		}
	}

	// Mount tmpfs before nested target directories so /tmp-host workloads
	// cannot inherit anything else from the host's /tmp.
	args = append(args, "--tmpfs", "/tmp")
	mounts := append(append([]string(nil), extra...), workspace)
	seenParents := map[string]bool{}
	for _, target := range mounts {
		for _, parent := range targetParents(target) {
			if seenParents[parent] || parent == "/" || parent == "/tmp" ||
				within(parent, "/usr") || within(parent, "/bin") ||
				within(parent, "/sbin") || within(parent, "/lib") ||
				within(parent, "/lib64") {
				continue
			}
			args = append(args, "--dir", parent)
			seenParents[parent] = true
		}
	}
	for _, source := range extra {
		args = append(args, "--ro-bind", source, source)
	}
	mount := "--ro-bind"
	if opts.Mode == "workspace" {
		mount = "--bind"
	}
	args = append(args, mount, workspace, workspace, "--chdir", workingDir)
	args = append(args, "--", executable)
	args = append(args, commandArgs...)
	// Never forward caller-controlled LD_PRELOAD, agent tokens or backend
	// credentials to the bwrap helper or guest by default.
	return bwrapPath, args, []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}, nil
}

func targetParents(path string) []string {
	parent := filepath.Dir(path)
	if parent == "/" || parent == "." {
		return nil
	}
	var out []string
	for parent != "/" && parent != "." {
		out = append(out, parent)
		parent = filepath.Dir(parent)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func within(path, base string) bool {
	if base == "/" {
		return filepath.IsAbs(path)
	}
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func executeSandbox(opts sandboxOptions, name string, args []string) error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("sandbox launch requires a non-root user; use --user to drop privileges first")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get sandbox working directory: %w", err)
	}
	binary, bwrapArgs, env, err := buildSandboxLaunch(opts, name, args, cwd)
	if err != nil {
		return err
	}
	return syscall.Exec(binary, append([]string{"bwrap"}, bwrapArgs...), env)
}
