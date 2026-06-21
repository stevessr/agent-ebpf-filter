package main

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AIToolIdentifier 识别 AI CLI 工具类型
type AIToolIdentifier struct {
	Name          string
	BinaryName    string
	CmdlineMarker []string
	VendorPath    string
}

var knownAITools = []AIToolIdentifier{
	{
		Name:          "Claude Code",
		BinaryName:    "node",
		CmdlineMarker: []string{"claude-code", "@cometix"},
		VendorPath:    ".local/share/fnm",
	},
	{
		Name:          "Codex",
		BinaryName:    "codex",
		CmdlineMarker: []string{"codex", "@openai"},
		VendorPath:    ".local/share/pnpm",
	},
	{
		Name:          "Cursor",
		BinaryName:    "node",
		CmdlineMarker: []string{"cursor", "@cursor"},
		VendorPath:    ".cursor",
	},
	{
		Name:          "GitHub Copilot CLI",
		BinaryName:    "node",
		CmdlineMarker: []string{"github-copilot-cli", "@githubnext"},
		VendorPath:    "",
	},
}

type AIToolProcess struct {
	Tool      AIToolIdentifier
	PID       int
	BinPath   string
	Cmdline   string
	HasSSL    bool
	SSLType   string
}

func identifyAIToolProcess(pid int, binPath, cmdline string) *AIToolProcess {
	baseName := filepath.Base(binPath)

	for _, tool := range knownAITools {
		if tool.BinaryName != baseName && baseName != tool.BinaryName {
			continue
		}

		matched := false
		for _, marker := range tool.CmdlineMarker {
			if strings.Contains(cmdline, marker) {
				matched = true
				break
			}
		}

		if !matched && tool.BinaryName == baseName {
			matched = tool.BinaryName == "codex"
		}

		if matched {
			hasSSL, sslType := detectSSLCapability(binPath)
			return &AIToolProcess{
				Tool:    tool,
				PID:     pid,
				BinPath: binPath,
				Cmdline: cmdline,
				HasSSL:  hasSSL,
				SSLType: sslType,
			}
		}
	}
	return nil
}

func detectSSLCapability(binPath string) (bool, string) {
	exe, err := elf.Open(binPath)
	if err != nil {
		return false, ""
	}
	defer exe.Close()

	symbols, _ := exe.Symbols()
	if symbols == nil {
		symbols, _ = exe.DynamicSymbols()
	}

	for _, sym := range symbols {
		if sym.Name == "SSL_write" || sym.Name == "SSL_read" {
			return true, "static-openssl"
		}
	}

	return false, ""
}

func scanForAITools() []AIToolProcess {
	var results []AIToolProcess

	entries, _ := filepath.Glob("/proc/[0-9]*/exe")
	for _, exeLink := range entries {
		pid := 0
		fmt.Sscanf(exeLink, "/proc/%d/exe", &pid)

		binPath, err := os.Readlink(exeLink)
		if err != nil || binPath == "" {
			continue
		}

		cmdlineBytes, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		cmdline := string(cmdlineBytes)

		if proc := identifyAIToolProcess(pid, binPath, cmdline); proc != nil {
			results = append(results, *proc)
		}
	}

	return results
}

func main() {
	fmt.Println("=== AI CLI Tools Scanner ===\n")

	tools := scanForAITools()
	if len(tools) == 0 {
		fmt.Println("No AI CLI tools detected")
		return
	}

	fmt.Printf("Found %d AI CLI tool process(es):\n\n", len(tools))
	for _, tool := range tools {
		fmt.Printf("Tool: %s\n", tool.Tool.Name)
		fmt.Printf("  PID: %d\n", tool.PID)
		fmt.Printf("  Binary: %s\n", tool.BinPath)
		fmt.Printf("  SSL Support: %v", tool.HasSSL)
		if tool.HasSSL {
			fmt.Printf(" (%s)", tool.SSLType)
		}
		fmt.Println()
		if len(tool.Cmdline) > 100 {
			fmt.Printf("  Cmdline: %s...\n", tool.Cmdline[:100])
		} else {
			fmt.Printf("  Cmdline: %s\n", tool.Cmdline)
		}
		fmt.Println()
	}
}
