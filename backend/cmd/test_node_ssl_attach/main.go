package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-ebpf-filter/internal/binaryresolver"
)

func main() {
	fmt.Println("=== Node.js SSL Attach Test ===")

	// 查找所有 Node.js 进程
	entries, err := filepath.Glob("/proc/[0-9]*/exe")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	nodeProcs := make(map[int]string)
	for _, exeLink := range entries {
		binPath, err := os.Readlink(exeLink)
		if err != nil || binPath == "" {
			continue
		}

		if !strings.Contains(binPath, "node") {
			continue
		}

		pid := 0
		fmt.Sscanf(exeLink, "/proc/%d/exe", &pid)
		if pid == 0 {
			continue
		}

		cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		nodeProcs[pid] = string(cmdline)
	}

	if len(nodeProcs) == 0 {
		fmt.Println("No Node.js processes found")
		return
	}

	fmt.Printf("Found %d Node.js processes:\n", len(nodeProcs))
	for pid, cmdline := range nodeProcs {
		isClaude := strings.Contains(cmdline, "claude-code") || strings.Contains(cmdline, "@cometix")
		isCodex := strings.Contains(cmdline, "codex") || strings.Contains(cmdline, "@openai")

		marker := ""
		if isClaude {
			marker = " [CLAUDE CODE]"
		} else if isCodex {
			marker = " [CODEX]"
		}

		exePath, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		fmt.Printf("  PID %d: %s%s\n", pid, exePath, marker)

		if isClaude || isCodex {
			// 测试 resolve
			resolved := binaryresolver.ResolveBinary(exePath, "")
			fmt.Printf("    StaticTLS: %v\n", resolved.StaticTLS)
		}
	}
}
