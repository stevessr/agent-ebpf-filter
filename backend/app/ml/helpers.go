package ml

import "strings"

// IsShell returns true if comm is a known shell binary.
func IsShell(comm string) bool {
	switch comm {
	case "bash", "zsh", "fish", "sh", "dash", "ksh", "tcsh", "ash":
		return true
	}
	return false
}

// IsPackageManager returns true if comm is a known package manager.
func IsPackageManager(comm string) bool {
	switch comm {
	case "apt", "apt-get", "yum", "dnf", "pacman", "zypper", "brew",
		"pip", "pip3", "npm", "yarn", "pnpm", "go", "cargo", "gem",
		"snap", "flatpak", "nix-env", "rpm", "dpkg":
		return true
	}
	return false
}

// IsAgentCLI returns true if comm is a known AI agent CLI.
func IsAgentCLI(comm string) bool {
	switch comm {
	case "claude", "gemini", "codex", "dsh", "pi", "omp", "kiro-cli", "gh", "cursor":
		return true
	}
	return false
}

// HasNetworkArgs returns true if args contain URL-like or host:port patterns.
func HasNetworkArgs(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") ||
			strings.HasPrefix(a, "ftp://") || strings.HasPrefix(a, "ws://") ||
			strings.Contains(a, ":") && !strings.Contains(a, "/") {
			return true
		}
	}
	return false
}

// HasFileArgs returns true if args contain file-like paths.
func HasFileArgs(args []string) bool {
	for _, a := range args {
		if strings.Contains(a, "/") && !strings.HasPrefix(a, "-") &&
			!strings.HasPrefix(a, "http") {
			return true
		}
	}
	return false
}

// HasRedirect returns true if args contain shell redirect operators.
func HasRedirect(args []string) bool {
	for _, a := range args {
		if a == ">" || a == ">>" || a == "<" || a == "2>" || a == "&>" || a == "|" {
			return true
		}
	}
	return false
}

// HasPipeChain returns true if args contain a pipe character.
func HasPipeChain(args []string) bool {
	for _, a := range args {
		if a == "|" {
			return true
		}
	}
	return false
}

// HasSudoInArgs returns true if args contain privilege-escalation commands.
func HasSudoInArgs(args []string) bool {
	for _, a := range args {
		if a == "sudo" || a == "doas" || a == "pkexec" {
			return true
		}
	}
	return false
}

// HasURLPattern returns true if s contains a URL.
func HasURLPattern(s string) bool {
	return strings.Contains(s, "http://") || strings.Contains(s, "https://") ||
		strings.Contains(s, "ftp://")
}

// HasIPPattern returns true if s contains IP-like patterns.
func HasIPPattern(s string) bool {
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

// HasEnvironmentVar returns true if args contain env var references.
func HasEnvironmentVar(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "$") || strings.Contains(a, "=${") {
			return true
		}
	}
	return false
}
