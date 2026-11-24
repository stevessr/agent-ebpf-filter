package ml

import (
	"net"
	"strconv"
	"strings"
)

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
	switch strings.ToLower(strings.TrimSpace(comm)) {
	case "claude", "gemini", "codex", "dsh", "pi", "omp", "kiro-cli", "gh", "cursor", "zg", "zcode", "zcode.appimage", "mcode", "minimax-code":
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

// HasRedirect returns true if args contain shell redirect operators,
// including compound tokens like ">/etc/passwd", "2>&1", or quoted pipelines.
func HasRedirect(args []string) bool {
	for _, a := range args {
		trimmed := strings.TrimSpace(a)
		if trimmed == "" {
			continue
		}
		if trimmed == ">" || trimmed == ">>" || trimmed == "<" || trimmed == "2>" || trimmed == "&>" || trimmed == "|" {
			return true
		}
		if strings.Contains(trimmed, ">>") || strings.Contains(trimmed, "2>") || strings.Contains(trimmed, "&>") || strings.Contains(trimmed, "<<") {
			return true
		}
		if strings.HasPrefix(trimmed, ">") || strings.HasPrefix(trimmed, "<") ||
			strings.Contains(trimmed, " >") || strings.Contains(trimmed, " <") ||
			strings.Contains(trimmed, ";>") || strings.Contains(trimmed, ";<") {
			return true
		}
		if hasSinglePipe(trimmed) {
			return true
		}
	}
	return false
}

// HasPipeChain returns true if args contain a pipe operator (excluding "||").
func HasPipeChain(args []string) bool {
	for _, a := range args {
		if hasSinglePipe(a) {
			return true
		}
	}
	return false
}

func hasSinglePipe(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '|' {
			prevPipe := i > 0 && s[i-1] == '|'
			nextPipe := i+1 < len(s) && s[i+1] == '|'
			if !prevPipe && !nextPipe {
				return true
			}
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

// HasIPPattern returns true if s contains a valid dotted-quad IPv4 address
// or a bracketed/colon-hex IPv6 literal.
func HasIPPattern(s string) bool {
	// Pass 1: preserve colons for IPv6 and host:port tokens.
	outerTokens := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', '/', '@', '=', ',', ';', '\'', '"', '(', ')', '<', '>':
			return true
		}
		return false
	})
	for _, tok := range outerTokens {
		if tok == "" {
			continue
		}
		if strings.HasPrefix(tok, "[") {
			if end := strings.IndexByte(tok, ']'); end > 1 {
				if ip := net.ParseIP(tok[1:end]); ip != nil {
					return true
				}
			}
		}
		if strings.Contains(tok, ":") {
			if host, _, err := net.SplitHostPort(tok); err == nil {
				if isStrictIPv4(host) || (strings.Contains(host, ":") && net.ParseIP(host) != nil) {
					return true
				}
			}
			trimmed := strings.Trim(tok, "[]")
			if strings.Count(trimmed, ":") >= 2 && net.ParseIP(trimmed) != nil {
				return true
			}
		}
	}

	// Pass 2: split on ':' as well (without splitting on '.') for IPv4 tokens.
	parts := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', ':', '/', '@', '=', ',', ';', '\'', '"', '(', ')', '[', ']', '<', '>':
			return true
		}
		return false
	})
	for _, p := range parts {
		if isStrictIPv4(p) {
			return true
		}
	}
	return false
}

func isStrictIPv4(token string) bool {
	if strings.Count(token, ".") != 3 {
		return false
	}
	octets := strings.Split(token, ".")
	if len(octets) != 4 {
		return false
	}
	for _, oct := range octets {
		if oct == "" || len(oct) > 3 {
			return false
		}
		for i := 0; i < len(oct); i++ {
			if oct[i] < '0' || oct[i] > '9' {
				return false
			}
		}
		v, err := strconv.Atoi(oct)
		if err != nil || v < 0 || v > 255 {
			return false
		}
	}
	return true
}

// ShellCompoundRiskScore inspects the joined command line for shell command
// chaining and indirection primitives across four categories:
//  1. command separators (;, &&, ||)
//  2. command substitution ($( or backtick)
//  3. shell function / alias redefinition (alias , () {)
//  4. dynamic eval / subshell blocks
//
// Returns matched categories / 4.0 bounded in [0, 1].
func ShellCompoundRiskScore(comm string, args []string) float64 {
	cmd := strings.TrimSpace(comm)
	if len(args) > 0 {
		joined := strings.Join(args, " ")
		if cmd != "" {
			cmd = cmd + " " + joined
		} else {
			cmd = joined
		}
	}
	if cmd == "" {
		return 0
	}
	lower := strings.ToLower(cmd)
	score := 0

	// 1. Command separators
	if strings.Contains(cmd, ";") || strings.Contains(cmd, "&&") || strings.Contains(cmd, "||") {
		score++
	}
	// 2. Command substitution
	if strings.Contains(cmd, "$(") || strings.Contains(cmd, "`") {
		score++
	}
	// 3. Function / alias redefinition
	if strings.HasPrefix(lower, "alias ") || strings.Contains(lower, " alias ") ||
		strings.Contains(lower, ";alias ") || strings.Contains(cmd, "() {") || strings.Contains(cmd, "(){") {
		score++
	}
	// 4. Dynamic eval / subshell execution blocks
	if strings.HasPrefix(lower, "eval ") || strings.Contains(lower, " eval ") ||
		strings.Contains(lower, ";eval ") || strings.Contains(cmd, "{ ") && strings.Contains(cmd, "; }") ||
		strings.Contains(cmd, "( ") && strings.Contains(cmd, " )") {
		score++
	}

	return float64(score) / 4.0
}

// ShellEvasionPrivilegeScore inspects the joined command line for general
// privilege, env-hijack, obfuscation, and destructive primitives:
//  1. Shell/loader env hijacking (BASH_ENV=, PROMPT_COMMAND=, LD_PRELOAD=, LD_LIBRARY_PATH=, ENV=)
//  2. Privilege/SUID escalation (chmod +s, chmod u+s, chmod 4755, chmod 4777, setuid, sudo -u, sudo )
//  3. Encoded/obfuscated execution (base64 -d, base64 --decode, xxd -r, printf '\x, printf "\x, \x)
//  4. Destructive disk/file erasure (shred, rm -rf, rm -fr, lvreduce, mkfs., dd if=, wipefs, > /etc/)
//
// Returns matched categories / 4.0 bounded in [0, 1].
func ShellEvasionPrivilegeScore(comm string, args []string) float64 {
	cmd := strings.TrimSpace(comm)
	if len(args) > 0 {
		joined := strings.Join(args, " ")
		if cmd != "" {
			cmd = cmd + " " + joined
		} else {
			cmd = joined
		}
	}
	if cmd == "" {
		return 0
	}
	lower := strings.ToLower(cmd)
	score := 0

	// 1. Shell / loader environment hijacking
	if strings.Contains(cmd, "BASH_ENV=") || strings.Contains(cmd, "PROMPT_COMMAND=") ||
		strings.Contains(cmd, "LD_PRELOAD=") || strings.Contains(cmd, "LD_LIBRARY_PATH=") ||
		strings.HasPrefix(cmd, "ENV=") || strings.Contains(cmd, " ENV=") || strings.Contains(cmd, ";ENV=") {
		score++
	}

	// 2. Privilege / SUID escalation
	if strings.Contains(lower, "chmod +s") || strings.Contains(lower, "chmod u+s") ||
		strings.Contains(lower, "chmod 4755") || strings.Contains(lower, "chmod 4777") ||
		strings.Contains(lower, "setuid") || strings.Contains(lower, "sudo -u") ||
		strings.HasPrefix(lower, "sudo ") || strings.Contains(lower, " sudo ") || strings.Contains(lower, ";sudo ") {
		score++
	}

	// 3. Encoded / obfuscated execution
	if strings.Contains(lower, "base64 -d") || strings.Contains(lower, "base64 --decode") ||
		strings.Contains(lower, "xxd -r") || strings.Contains(lower, "printf '\\x") ||
		strings.Contains(lower, `printf "\x`) || strings.Contains(lower, `\x`) {
		score++
	}

	// 4. Destructive disk/file erasure or sensitive overwrite
	if strings.HasPrefix(lower, "shred") || strings.Contains(lower, " shred ") || strings.Contains(lower, ";shred ") ||
		strings.Contains(lower, "rm -rf") || strings.Contains(lower, "rm -fr") ||
		strings.Contains(lower, "lvreduce") || strings.Contains(lower, "mkfs.") ||
		strings.Contains(lower, "dd if=") || strings.Contains(lower, "wipefs") ||
		strings.Contains(lower, "> /etc/") || strings.Contains(lower, ">/etc/") {
		score++
	}

	return float64(score) / 4.0
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
