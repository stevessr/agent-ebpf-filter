package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-ebpf-filter/internal/binaryresolver"
)

func main() {
	fmt.Println("=== SSL Discovery Test ===")
	fmt.Println()

	// 测试 1: 检查 Node.js
	nodePath := "/run/user/1000/fnm_multishells/15125_1781187621992/bin/node"
	if _, err := os.Stat(nodePath); err == nil {
		fmt.Printf("1. Testing Node.js: %s\n", nodePath)
		resolved := binaryresolver.ResolveBinary(nodePath, "")
		fmt.Printf("   StaticTLS: %v\n", resolved.StaticTLS)
		fmt.Printf("   RealPath: %s\n", resolved.RealPath)
		if resolved.Error != "" {
			fmt.Printf("   Error: %s\n", resolved.Error)
		}
		fmt.Println()
	}

	// 测试 2: 查找所有 Codex 二进制文件
	fmt.Println("2. Searching for Codex binaries:")
	codexBins, _ := filepath.Glob("/home/steve/.local/share/pnpm/store/v11/links/@openai/codex/*/*/node_modules/@openai/codex/vendor/x86_64-unknown-linux-musl/bin/codex")
	for _, codexPath := range codexBins {
		if _, err := os.Stat(codexPath); err == nil {
			fmt.Printf("   Found: %s\n", codexPath)
			resolved := binaryresolver.ResolveBinary(codexPath, "")
			fmt.Printf("   StaticTLS: %v\n", resolved.StaticTLS)
			if resolved.Error != "" {
				fmt.Printf("   Error: %s\n", resolved.Error)
			}
			break
		}
	}
	fmt.Println()

	// 测试 3: 扫描所有正在运行的相关进程
	fmt.Println("3. Scanning running processes:")
	entries, _ := filepath.Glob("/proc/[0-9]*/exe")
	found := 0
	for _, exeLink := range entries {
		binPath, err := os.Readlink(exeLink)
		if err != nil || binPath == "" {
			continue
		}

		baseName := filepath.Base(binPath)
		if baseName != "node" && baseName != "codex" {
			continue
		}

		pid := 0
		fmt.Sscanf(exeLink, "/proc/%d/exe", &pid)

		cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		cmdStr := string(cmdline)

		isTarget := strings.Contains(cmdStr, "claude-code") ||
		            strings.Contains(cmdStr, "codex") ||
		            strings.Contains(cmdStr, "@cometix") ||
		            strings.Contains(cmdStr, "@openai") ||
		            baseName == "codex"

		if !isTarget {
			continue
		}

		found++
		fmt.Printf("   PID %d: %s\n", pid, binPath)

		resolved := binaryresolver.ResolveBinary(binPath, "")
		fmt.Printf("   StaticTLS: %v\n", resolved.StaticTLS)

		if strings.Contains(cmdStr, "claude-code") || strings.Contains(cmdStr, "@cometix") {
			fmt.Printf("   Type: Claude Code (Node.js)\n")
		} else if strings.Contains(cmdStr, "codex") || strings.Contains(cmdStr, "@openai") || baseName == "codex" {
			fmt.Printf("   Type: Codex (native binary)\n")
		}
		fmt.Println()
	}

	if found == 0 {
		fmt.Println("   No Claude Code or Codex processes found")
	}

	fmt.Printf("\nTotal target processes found: %d\n", found)
}
