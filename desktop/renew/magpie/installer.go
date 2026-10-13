// Package magpie installs Apache Magpie agent plugins with official agent CLIs.
// It never runs with eBPF/backend privileges or interprets arbitrary shell text.
package magpie

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const Marketplace = "apache/magpie"

const (
	Claude   = "Claude Code"
	Codex    = "Codex CLI"
	Baseline = "仅安装基础组件"
	PR       = "PR 审查"
	Release  = "发布管理"
	Security = "安全响应"
	Issue    = "Issue 管理"
	Health   = "仓库健康"
	Growth   = "贡献者成长"
	Mentor   = "导师辅导"
	Pairing  = "结对协作"
)

var Options = []string{Baseline, PR, Release, Security, Issue, Health, Growth, Mentor, Pairing}

var familyIDs = map[string]string{
	PR:       "magpie-pr-management",
	Release:  "magpie-release-management",
	Security: "magpie-security",
	Issue:    "magpie-issue",
	Health:   "magpie-repo-health",
	Growth:   "magpie-contributor-growth",
	Mentor:   "magpie-mentoring",
	Pairing:  "magpie-pairing",
}

// Step contains executable and argv; commands never pass through a shell.
type Step struct {
	Executable string
	Arguments  []string
}

func (s Step) Display() string {
	return s.Executable + " " + strings.Join(s.Arguments, " ")
}

func binaryFor(target string) (string, error) {
	switch target {
	case Claude:
		return "claude", nil
	case Codex:
		return "codex", nil
	default:
		return "", fmt.Errorf("不支持的 Agent: %q", target)
	}
}

// Plan limits executable names and plugin identifiers to a reviewed allowlist.
func Plan(target, family string) ([]Step, error) {
	binary, err := binaryFor(target)
	if err != nil {
		return nil, err
	}
	if family != Baseline && familyIDs[family] == "" {
		return nil, fmt.Errorf("不支持的 Magpie 插件: %q", family)
	}
	steps := []Step{{binary, []string{"plugin", "marketplace", "add", Marketplace}}}
	plugins := []string{"magpie-setup"}
	if target == Claude {
		plugins = append(plugins, "magpie-agent-guard")
	}
	plugins = append(plugins, "magpie-utilities")
	if family != Baseline {
		plugins = append(plugins, familyIDs[family])
	}
	verb := "install"
	if target == Codex {
		verb = "add"
	}
	for _, id := range plugins {
		steps = append(steps, Step{binary, []string{"plugin", verb, id + "@apache-magpie"}})
	}
	return steps, nil
}

type Executor interface {
	Run(ctx context.Context, executable string, args ...string) error
}

type CLIExecutor struct{}

type cappedOutput struct{ data []byte }

func (o *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	const limit = 16 << 10
	if len(o.data) < limit {
		remaining := limit - len(o.data)
		if n < remaining {
			remaining = n
		}
		o.data = append(o.data, p[:remaining]...)
	}
	return n, nil
}

// Some CLIs return a nonzero exit status when a plugin is already present.
func alreadyRegistered(args []string, output string) bool {
	if len(args) < 3 || args[0] != "plugin" {
		return false
	}
	if !(len(args) == 4 && args[1] == "marketplace" && args[2] == "add") &&
		!(len(args) == 3 && (args[1] == "install" || args[1] == "add")) {
		return false
	}
	lower := strings.ToLower(output)
	return strings.Contains(lower, "already installed") ||
		strings.Contains(lower, "already added") ||
		strings.Contains(lower, "already exists")
}

func (CLIExecutor) Run(ctx context.Context, executable string, args ...string) error {
	binary, err := exec.LookPath(executable)
	if err != nil {
		return fmt.Errorf("未找到 %s；请先安装相应 Agent CLI", executable)
	}
	stepCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	command := exec.CommandContext(stepCtx, binary, args...)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	command.Dir = filepath.Clean(home)
	// Bound output to prevent runaway logs. Never send CLI output (which may
	// contain secrets) to the desktop UI or privileged eBPF process.
	var output cappedOutput
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		if stepCtx.Err() == nil && alreadyRegistered(args, string(output.data)) {
			return nil
		}
		if stepCtx.Err() != nil {
			return fmt.Errorf("%s 命令超时或已取消: %w", executable, stepCtx.Err())
		}
		return fmt.Errorf("%s 插件操作失败: %w", executable, err)
	}
	return nil
}

// Install is called only after an explicit confirmation in the native UI.
func Install(ctx context.Context, runner Executor, target, family string) error {
	if os.Geteuid() == 0 {
		return errors.New("不能以 root 身份安装 Agent 用户插件；请以普通用户启动 Renew")
	}
	steps, err := Plan(target, family)
	if err != nil {
		return err
	}
	if runner == nil {
		runner = CLIExecutor{}
	}
	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := runner.Run(ctx, step.Executable, step.Arguments...); err != nil {
			return fmt.Errorf("%s: %w", step.Display(), err)
		}
	}
	return nil
}

func Available(target string) bool {
	binary, err := binaryFor(target)
	if err != nil {
		return false
	}
	_, err = exec.LookPath(binary)
	return err == nil
}
