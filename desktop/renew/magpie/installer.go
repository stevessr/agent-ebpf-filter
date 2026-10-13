// Package magpie integrates yetone/magpie's CLI with the unprivileged Renew UI.
// Plugin packages execute third-party code. Only explicit user actions may invoke
// these commands, and they never pass through the privileged eBPF backend.
package magpie

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	ProjectURL = "https://github.com/yetone/magpie"
	InstallModule = "github.com/yetone/magpie@latest"
	InstallApp = "install-app"
	AddPlugin = "add-plugin"
	UpdatePlugins = "update-plugins"
)

var packagePattern = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*(?:@[a-zA-Z0-9][a-zA-Z0-9._-]*)?$`)

// ValidPackage permits npm package specifications only, not local paths,
// git URLs, shell switches, or arbitrary plugin download endpoints.
func ValidPackage(spec string) bool {
	return len(spec) > 0 && len(spec) <= 160 &&
		!strings.Contains(spec, "..") &&
		!strings.Contains(spec, "//") &&
		!strings.ContainsAny(spec, "\\\n\r\t ") &&
		packagePattern.MatchString(spec)
}

type Step struct {
	Program string
	Args []string
	Timeout time.Duration
}

func (s Step) Display() string {
	return s.Program + " " + strings.Join(s.Args, " ")
}

func Plan(action, spec string) (Step, error) {
	switch action {
	case InstallApp:
		return Step{"go", []string{"install", InstallModule}, 12 * time.Minute}, nil
	case AddPlugin:
		if !ValidPackage(spec) {
			return Step{}, errors.New("只接受 npm 包名（例如 @magpie-community/middleware-model-map），不支持路径、URL 或命令参数")
		}
		return Step{"magpie", []string{"plugin", "add", spec}, 3 * time.Minute}, nil
	case UpdatePlugins:
		return Step{"magpie", []string{"plugin", "update"}, 6 * time.Minute}, nil
	default:
		return Step{}, fmt.Errorf("不支持的 Magpie 操作: %q", action)
	}
}

// Locate returns the CLI path without starting magpie or accessing accounts.
// go install may place it outside PATH.
func Locate() string {
	if p, err := exec.LookPath("magpie"); err == nil {
		return p
	}
	candidates := []string{}
	if bin := os.Getenv("GOBIN"); filepath.IsAbs(bin) {
		candidates = append(candidates, filepath.Join(bin, binaryName()))
	}
	for _, part := range filepath.SplitList(os.Getenv("GOPATH")) {
		if filepath.IsAbs(part) {
			candidates = append(candidates, filepath.Join(part, "bin", binaryName()))
			break
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "go", "bin", binaryName()))
	}
	for _, path := range candidates {
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			return path
		}
	}
	return ""
}

func binaryName() string {
	if os.PathSeparator == '\\' {
		return "magpie.exe"
	}
	return "magpie"
}

func GoAvailable() bool {
	_, err := exec.LookPath("go")
	return err == nil
}

// Executor allows deterministic tests and keeps process execution isolated.
type Executor interface {
	Run(ctx context.Context, step Step) error
}

type CLIExecutor struct{}

type boundedOutput struct {
	buffer []byte
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	const limit = 16 << 10
	if len(b.buffer) < limit {
		remaining := limit - len(b.buffer)
		if remaining > n {
			remaining = n
		}
		b.buffer = append(b.buffer, p[:remaining]...)
	}
	return n, nil
}

// Run never uses a shell, sudo or pkexec; stdout/stderr stay private so a
// plugin cannot accidentally expose credentials in the monitoring interface.
func (CLIExecutor) Run(ctx context.Context, step Step) error {
	program := step.Program
	if program == "magpie" {
		program = Locate()
		if program == "" {
			return errors.New("未检测到 magpie 命令，请先安装 Magpie")
		}
	} else if program != "go" {
		return errors.New("拒绝执行非预期程序")
	}
	if program == "go" && !GoAvailable() {
		return errors.New("未找到 Go 工具链，请先安装 Go 或从官网安装 Magpie")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, step.Timeout)
	defer cancel()
	cmd := exec.CommandContext(deadline, program, step.Args...)
	cmd.Dir = home
	cmd.Stdin = strings.NewReader("")
	cmd.Env = append(os.Environ(), "MAGPIE_NO_ACCOUNTS=1", "DO_NOT_TRACK=1")
	output := &boundedOutput{}
	cmd.Stdout = io.Writer(output)
	cmd.Stderr = io.Writer(output)
	if err := cmd.Run(); err != nil {
		if deadline.Err() != nil {
			return fmt.Errorf("安装进程超时或取消: %w", deadline.Err())
		}
		return fmt.Errorf("本地命令执行失败（请检查 Magpie/Go 环境）: %w", err)
	}
	return nil
}

// Execute is called only after the native UI's explicit confirmation.
func Execute(ctx context.Context, runner Executor, action, spec string) error {
	step, err := Plan(action, spec)
	if err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		return errors.New("请以普通用户运行 Renew，不能在 root 会话内安装 Magpie 或第三方插件")
	}
	if runner == nil {
		runner = CLIExecutor{}
	}
	return runner.Run(ctx, step)
}
