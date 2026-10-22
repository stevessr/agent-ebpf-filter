// Package heuristics provides conservative, deterministic process-behavior rules.
// A language runtime alone is never proof that it is serving HTTP.
package heuristics

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxProcCommandBytes = 8192

// ShellFinding is evidence for a shell directly requested by a web worker.
// It is not evidence that a request came from an external client.
type ShellFinding struct {
	Code      string
	Runtime   string
	Shell     string
	ParentPID uint32
}

func (f ShellFinding) Reason() string {
	return fmt.Sprintf("%s web-service process (PID %d) attempted to spawn shell %q; web request handlers normally do not require interactive command interpreters", f.Runtime, f.ParentPID, f.Shell)
}

// ShellExecutable recognizes actual shell entrypoints, not similarly named tools
// such as shellcheck. env and busybox must explicitly select a shell.
func ShellExecutable(command string, args []string) string {
	name := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
	switch name {
	case "sh", "bash", "dash", "ash", "zsh", "fish", "ksh", "mksh", "csh", "tcsh", "pwsh":
		return name
	case "busybox":
		if len(args) > 0 {
			return ShellExecutable(args[0], nil)
		}
	case "env":
		for i := 0; i < len(args); i++ {
			arg := args[i]
			if arg == "-S" || arg == "--split-string" {
				if i+1 < len(args) {
					fields := strings.Fields(args[i+1])
					if len(fields) > 0 {
						return ShellExecutable(fields[0], fields[1:])
					}
				}
				return ""
			}
			// -u / --unset consumes a following environment variable.
			if arg == "-u" || arg == "--unset" {
				i++
				continue
			}
			if strings.HasPrefix(arg, "-") || strings.Contains(arg, "=") {
				continue
			}
			return ShellExecutable(arg, args[i+1:])
		}
	}
	return ""
}

func processName(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	return strings.ToLower(filepath.Base(strings.TrimSpace(argv[0])))
}

func hasExactArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func hasArg(args []string, target string) bool {
	for _, arg := range args {
		if strings.EqualFold(arg, target) {
			return true
		}
	}
	return false
}

func hasModule(args []string, modules ...string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "-m" {
			continue
		}
		for _, module := range modules {
			if strings.EqualFold(args[i+1], module) {
				return true
			}
		}
	}
	return false
}

func webEntrypoint(args []string) bool {
	for _, arg := range args {
		base := strings.ToLower(filepath.Base(arg))
		for _, prefix := range []string{"server.", "http-server.", "api-server.", "web-server.", "serve."} {
			if strings.HasPrefix(base, prefix) &&
				(strings.HasSuffix(base, ".js") || strings.HasSuffix(base, ".mjs") ||
					strings.HasSuffix(base, ".cjs") || strings.HasSuffix(base, ".ts") ||
					strings.HasSuffix(base, ".mts") || strings.HasSuffix(base, ".tsx")) {
				return true
			}
		}
	}
	return false
}

func hasEntrypoint(args []string, entry string) bool {
	for _, arg := range args {
		if strings.EqualFold(filepath.Base(arg), entry) {
			return true
		}
	}
	return false
}

// IdentifyWebService only returns a runtime when argv contains positive
// server evidence. Plain python scripts, node tools and php CLI remain unclassified.
func IdentifyWebService(argv []string) string {
	name := processName(argv)
	if name == "" {
		return ""
	}
	args := argv[1:]
	switch {
	case strings.HasPrefix(name, "php-fpm"), strings.HasPrefix(name, "php-cgi"):
		return "PHP"
	case name == "php" || strings.HasPrefix(name, "php8.") || strings.HasPrefix(name, "php7."):
		if hasExactArg(args, "-S") || (hasArg(args, "artisan") && hasArg(args, "serve")) ||
			hasArg(args, "server:run") {
			return "PHP"
		}
	case name == "uvicorn" || name == "gunicorn" || name == "hypercorn" ||
		name == "daphne" || name == "waitress-serve" || name == "granian":
		return "Python"
	case strings.HasPrefix(name, "python") || name == "pypy" || name == "pypy3":
		if hasModule(args, "uvicorn", "gunicorn", "hypercorn", "daphne", "http.server", "waitress", "fastapi") ||
			(hasModule(args, "flask") && (hasArg(args, "run") || hasArg(args, "serve"))) ||
			(hasArg(args, "manage.py") && hasArg(args, "runserver")) ||
			(hasArg(args, "flask") && (hasArg(args, "run") || hasArg(args, "serve"))) {
			return "Python"
		}
	case name == "flask", name == "fastapi":
		if hasArg(args, "run") || hasArg(args, "serve") || hasArg(args, "dev") {
			return "Python"
		}
	case name == "node" || name == "nodejs" || name == "tsx" ||
		name == "ts-node" || name == "bun" || name == "deno":
		if webEntrypoint(args) ||
			(hasEntrypoint(args, "next") && (hasArg(args, "start") || hasArg(args, "dev"))) ||
			(hasEntrypoint(args, "nuxt") && (hasArg(args, "start") || hasArg(args, "dev"))) ||
			(hasEntrypoint(args, "nest") && hasArg(args, "start")) {
			return "Node.js/TypeScript"
		}
	case name == "next" || name == "nuxt" || name == "nest" ||
		name == "http-server" || name == "vite-node":
		if name == "http-server" || hasArg(args, "start") || hasArg(args, "dev") {
			return "Node.js/TypeScript"
		}
	}
	return ""
}

// MatchWebShellSpawn is pure and can be tested without procfs/eBPF.
func MatchWebShellSpawn(command string, args, parentArgv []string, parentPID uint32) (ShellFinding, bool) {
	shell := ShellExecutable(command, args)
	if shell == "" {
		return ShellFinding{}, false
	}
	runtime := IdentifyWebService(parentArgv)
	if runtime == "" {
		return ShellFinding{}, false
	}
	code := "JS_WEB_SHELL_SPAWN"
	switch runtime {
	case "PHP":
		code = "PHP_WEB_SHELL_SPAWN"
	case "Python":
		code = "PYTHON_WEB_SHELL_SPAWN"
	}
	return ShellFinding{Code: code, Runtime: runtime, Shell: shell, ParentPID: parentPID}, true
}

// ReadProcCommand reads only a small, bounded amount of local process identity.
// An unavailable/oversized proc entry is unknown, never a match.
func ReadProcCommand(pid uint32) ([]string, error) {
	if pid <= 1 {
		return nil, errors.New("missing process pid")
	}
	f, err := os.Open(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxProcCommandBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxProcCommandBytes {
		return nil, errors.New("process cmdline too large")
	}
	var args []string
	for _, part := range bytes.Split(data, []byte{0}) {
		if len(part) > 0 {
			args = append(args, string(part))
		}
	}
	if len(args) > 0 {
		return args, nil
	}
	// A PHP-FPM worker may report only a thread/process title.
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(string(comm))
	if name == "" {
		return nil, errors.New("empty process identity")
	}
	return []string{name}, nil
}

// ReadProcParentPID handles a parenthesized comm containing spaces or ')'.
func ReadProcParentPID(pid uint32) (uint32, error) {
	if pid <= 1 {
		return 0, errors.New("missing process pid")
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	end := bytes.LastIndexByte(data, ')')
	if end < 0 {
		return 0, errors.New("invalid process stat")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 2 {
		return 0, errors.New("invalid parent stat")
	}
	n, err := strconv.ParseUint(fields[1], 10, 32)
	if err != nil || n <= 1 || uint32(n) == pid {
		return 0, errors.New("invalid parent pid")
	}
	return uint32(n), nil
}

// DetectWebShellSpawn checks a directly observed parent PID. It is evidence for
// event-time alerts; by itself it cannot prevent an execve syscall.
func DetectWebShellSpawn(command string, args []string, parentPID uint32) (ShellFinding, bool) {
	if ShellExecutable(command, args) == "" {
		return ShellFinding{}, false
	}
	argv, err := ReadProcCommand(parentPID)
	if err != nil {
		return ShellFinding{}, false
	}
	return MatchWebShellSpawn(command, args, argv, parentPID)
}

// DetectWebShellWrapper checks the wrapper's direct parent, or the supplied
// PID itself when the caller provides a web-server process PID. This can be
// enforced only when command execution actually passes through the wrapper.
func DetectWebShellWrapper(command string, args []string, wrapperPID uint32) (ShellFinding, bool) {
	if ShellExecutable(command, args) == "" {
		return ShellFinding{}, false
	}
	// The direct parent is preferred: the wrapper normally carries its own PID.
	if parentPID, err := ReadProcParentPID(wrapperPID); err == nil {
		if finding, ok := DetectWebShellSpawn(command, args, parentPID); ok {
			return finding, true
		}
	}
	return DetectWebShellSpawn(command, args, wrapperPID)
}
