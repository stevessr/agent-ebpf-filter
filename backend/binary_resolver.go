package main

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const binaryResolverMaxShebangBytes = 256

type ResolvedBinary struct {
	Input           string `json:"input"`
	Path            string `json:"path"`
	RealPath        string `json:"realPath,omitempty"`
	Shebang         string `json:"shebang,omitempty"`
	ShebangTarget   string `json:"shebangTarget,omitempty"`
	StaticTLS       bool   `json:"staticTls"`
	ContainerTarget string `json:"containerTarget,omitempty"`
	ContainerPID    int    `json:"containerPid,omitempty"`
	ContainerRootFS string `json:"containerRootfs,omitempty"`
	Error           string `json:"error,omitempty"`
}

func ResolveBinary(input string, envPath string) ResolvedBinary {
	result := ResolvedBinary{Input: strings.TrimSpace(input)}
	if result.Input == "" {
		result.Error = "empty binary input"
		return result
	}
	if container, inner, ok := splitContainerBinaryTarget(result.Input); ok {
		result.ContainerTarget = container
		result.Input = inner
		result.ContainerPID, result.ContainerRootFS = inspectDockerContainerRoot(container)
		if result.ContainerRootFS != "" && filepath.IsAbs(inner) {
			result.Input = filepath.Join(result.ContainerRootFS, strings.TrimPrefix(inner, string(filepath.Separator)))
		}
	}
	path, err := resolveBinaryPath(result.Input, envPath)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Path = path
	if realPath, err := filepath.EvalSymlinks(path); err == nil {
		result.RealPath = realPath
	} else {
		result.RealPath = path
	}
	result.Shebang, result.ShebangTarget = readBinaryShebang(result.RealPath)
	result.StaticTLS = binaryHasStaticTLS(result.RealPath)
	return result
}

func splitContainerBinaryTarget(input string) (string, string, bool) {
	if rest, ok := strings.CutPrefix(input, "docker://"); ok {
		container, inner, found := strings.Cut(rest, ":")
		if found && strings.TrimSpace(container) != "" && strings.TrimSpace(inner) != "" {
			return container, inner, true
		}
	}
	if rest, ok := strings.CutPrefix(input, "docker:"); ok {
		container, inner, found := strings.Cut(rest, ":")
		if found && strings.TrimSpace(container) != "" && strings.TrimSpace(inner) != "" {
			return container, inner, true
		}
	}
	return "", input, false
}

func inspectDockerContainerRoot(container string) (int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{json .State.Pid}} {{json .GraphDriver.Data.MergedDir}}", container)
	out, err := cmd.Output()
	if err != nil {
		return 0, ""
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, ""
	}
	var pid int
	var root string
	_ = json.Unmarshal([]byte(fields[0]), &pid)
	_ = json.Unmarshal([]byte(fields[1]), &root)
	if root == "" && pid > 0 {
		root = filepath.Join("/proc", strconvFormatInt(int64(pid)), "root")
	}
	return pid, root
}

func resolveBinaryPath(input string, envPath string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("empty binary path")
	}
	if strings.ContainsRune(input, filepath.Separator) {
		if filepath.IsAbs(input) {
			return input, nil
		}
		abs, err := filepath.Abs(input)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	if envPath == "" {
		envPath = os.Getenv("PATH")
	}
	for _, dir := range filepath.SplitList(envPath) {
		candidate := filepath.Join(dir, input)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate, nil
		}
	}
	return "", errors.New("binary not found in PATH")
}

func readBinaryShebang(path string) (string, string) {
	file, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer file.Close()
	buf := make([]byte, binaryResolverMaxShebangBytes)
	n, err := file.Read(buf)
	if err != nil || n < 3 || !bytes.HasPrefix(buf[:n], []byte("#!")) {
		return "", ""
	}
	line := strings.TrimSpace(strings.SplitN(string(buf[:n]), "\n", 2)[0])
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return line, ""
	}
	return line, fields[0]
}

func binaryHasStaticTLS(path string) bool {
	file, err := elf.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	for _, section := range file.Sections {
		name := strings.ToLower(section.Name)
		if name == ".tdata" || name == ".tbss" {
			return true
		}
	}
	for _, program := range file.Progs {
		if program.Type == elf.PT_TLS {
			return true
		}
	}
	return false
}
