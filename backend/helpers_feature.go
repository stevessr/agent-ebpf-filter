package main

import (
	"math"
	"strings"
)

// ── Helper functions ──

func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func isShell(comm string) bool {
	shells := map[string]bool{"bash": true, "zsh": true, "fish": true, "sh": true, "dash": true, "ksh": true, "tcsh": true, "ash": true}
	return shells[comm]
}

func isPackageManager(comm string) bool {
	pms := map[string]bool{
		"apt": true, "apt-get": true, "yum": true, "dnf": true, "pacman": true,
		"zypper": true, "brew": true, "pip": true, "pip3": true, "npm": true,
		"yarn": true, "pnpm": true, "go": true, "cargo": true, "gem": true,
		"snap": true, "flatpak": true, "nix-env": true, "rpm": true, "dpkg": true,
	}
	return pms[comm]
}

func isAgentCLI(comm string) bool {
	agents := map[string]bool{
		"claude": true, "gemini": true, "codex": true, "kiro-cli": true,
		"gh": true, "cursor": true,
	}
	return agents[comm]
}

func hasNetworkArgs(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") ||
			strings.HasPrefix(a, "ftp://") || strings.HasPrefix(a, "ws://") ||
			strings.Contains(a, ":") && !strings.Contains(a, "/") {
			return true
		}
	}
	return false
}

func hasFileArgs(args []string) bool {
	for _, a := range args {
		if strings.Contains(a, "/") && !strings.HasPrefix(a, "-") &&
			!strings.HasPrefix(a, "http") {
			return true
		}
	}
	return false
}

func hasRedirect(args []string) bool {
	for _, a := range args {
		if a == ">" || a == ">>" || a == "<" || a == "2>" || a == "&>" || a == "|" {
			return true
		}
	}
	return false
}

func hasPipeChain(args []string) bool {
	for _, a := range args {
		if a == "|" {
			return true
		}
	}
	return false
}

func hasSudoInArgs(args []string) bool {
	for _, a := range args {
		if a == "sudo" || a == "doas" || a == "pkexec" {
			return true
		}
	}
	return false
}

func hasURLPattern(s string) bool {
	return strings.Contains(s, "http://") || strings.Contains(s, "https://") ||
		strings.Contains(s, "ftp://")
}

func hasIPPattern(s string) bool {
	// Simple IP-like pattern check
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ':' || r == '/' || r == '@' || r == '.'
	})
	for _, p := range parts {
		if len(p) > 0 && (strings.Count(p, ".") == 3 || strings.Count(p, ".") == 4) {
			return true
		}
	}
	return false
}

func hasEnvironmentVar(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "$") || strings.Contains(a, "=${") {
			return true
		}
	}
	return false
}

// shannonEntropy computes Shannon entropy of a string (0-1 normalized)
func shannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	counts := make(map[byte]int)
	for i := 0; i < len(s); i++ {
		counts[s[i]]++
	}
	var entropy float64
	n := float64(len(s))
	for _, c := range counts {
		p := float64(c) / n
		entropy -= p * math.Log2(p)
	}
	// Normalize by log2(256) = 8
	return entropy / 8.0
}
