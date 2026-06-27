// backend-test-watcher — 监听 backend/ 目录 .go 文件变动自动运行 go test
// 用法: cd backend && go run ../scripts/backend-test-watcher.go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	// 定位项目根目录（从脚本位置上溯）
	scriptDir := filepath.Dir(os.Args[0])
	projectDir := filepath.Dir(scriptDir) // scripts/ 的上一级
	backendDir := filepath.Join(projectDir, "backend")

	// 如果调用时不在项目目录，尝试从 pwd 推断
	if fi, err := os.Stat(filepath.Join(backendDir, "go.mod")); err != nil || fi == nil {
		cwd, _ := os.Getwd()
		if fi, err := os.Stat(filepath.Join(cwd, "backend", "go.mod")); err == nil && fi != nil {
			projectDir = cwd
			backendDir = filepath.Join(projectDir, "backend")
		} else if fi, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil && fi != nil {
			projectDir = cwd
			backendDir = filepath.Join(projectDir, "backend")
		}
	}

	fmt.Printf("[test-watcher] 项目目录: %s\n", projectDir)
	fmt.Printf("[test-watcher] 监听目录: %s/**/*.go\n", backendDir)
	fmt.Printf("[test-watcher] PID: %d\n\n", os.Getpid())

	// 先跑一次完整测试做基线
	fmt.Println("=== 初始测试 ===")
	runTests(backendDir)
	fmt.Println("================\n")

	// 用 stat + 轮询的方式检测文件变更（零依赖）
	pollInterval := 3 * time.Second

	// 记录所有 .go 文件的 mtime
	lastMtimes := make(map[string]time.Time)
	filepath.Walk(backendDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".go") {
			lastMtimes[path] = info.ModTime()
		}
		return nil
	})

	fmt.Println("[test-watcher] 监听已就绪，等待文件变动...\n")

	for {
		time.Sleep(pollInterval)

		changed := false
		var changedFiles []string

		filepath.Walk(backendDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			mt := info.ModTime()
			if prev, ok := lastMtimes[path]; !ok || !mt.Equal(prev) {
				lastMtimes[path] = mt
				if !changed {
					changed = true
				}
				if len(changedFiles) < 5 {
					rel, _ := filepath.Rel(backendDir, path)
					changedFiles = append(changedFiles, rel)
				}
			}
			return nil
		})

		if changed {
			now := time.Now().Format("15:04:05")
			fmt.Printf("[test-watcher] %s 检测到 %d 个文件变更:\n", now, len(changedFiles))
			for _, f := range changedFiles {
				fmt.Printf("  > %s\n", f)
			}
			fmt.Println()
			runTests(backendDir)
			fmt.Println("────────────────────────────────────────")
		}
	}
}

func runTests(backendDir string) {
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = backendDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start).Round(time.Millisecond)

	if err == nil {
		fmt.Printf("\n[test-watcher] ✅ %s 所有测试通过 (%v)\n", time.Now().Format("15:04:05"), elapsed)
	} else {
		fmt.Printf("\n[test-watcher] ❌ %s 测试失败 (%v)\n", time.Now().Format("15:04:05"), elapsed)
	}
}
