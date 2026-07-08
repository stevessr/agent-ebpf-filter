package rules

import (
	"agent-ebpf-filter/redaction"
	"path/filepath"
	"strings"
)

// RedactPath applies redaction rules to a filesystem path string.
//
// Rules:
// - Basic: no path redaction
// - Standard: replace /home/user with ~ and normalize config directories
// - Strict: keep only filename and top-level directory
func RedactPath(path string, level redaction.RedactionLevel) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}

	switch level {
	case redaction.RedactionLevelNone, redaction.RedactionLevelBasic:
		return path
	case redaction.RedactionLevelStandard:
		path = normalizeHomePath(path)
		path = normalizeConfigPath(path)
		return path
	case redaction.RedactionLevelStrict:
		return redactPathStrict(path)
	default:
		return path
	}
}

// normalizeHomePath replaces concrete home directories with ~.
// Examples:
//   /home/user/file.txt -> ~/file.txt
//   /Users/alice/.ssh/id_rsa -> ~/.ssh/id_rsa
func normalizeHomePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}

	cleaned := filepath.Clean(path)
	if cleaned == "." {
		return path
	}

	if strings.HasPrefix(cleaned, "/home/") {
		parts := strings.Split(cleaned, string(filepath.Separator))
		if len(parts) >= 4 {
			rest := strings.Join(parts[4:], string(filepath.Separator))
			if rest == "" {
				return "~"
			}
			return filepath.ToSlash(filepath.Join("~", rest))
		}
	}

	if strings.HasPrefix(cleaned, "/Users/") {
		parts := strings.Split(cleaned, string(filepath.Separator))
		if len(parts) >= 4 {
			rest := strings.Join(parts[4:], string(filepath.Separator))
			if rest == "" {
				return "~"
			}
			return filepath.ToSlash(filepath.Join("~", rest))
		}
	}

	if strings.HasPrefix(cleaned, "~/") || cleaned == "~" {
		return filepath.ToSlash(cleaned)
	}

	return filepath.ToSlash(cleaned)
}

// normalizeConfigPath normalizes common configuration directories while preserving
// the useful tail of the path.
func normalizeConfigPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}

	cleaned := filepath.ToSlash(filepath.Clean(path))
	if cleaned == "." {
		return path
	}

	homeNormalized := normalizeHomePath(cleaned)
	if homeNormalized != cleaned {
		cleaned = homeNormalized
	}

	replacements := []struct {
		prefix string
		alias  string
	}{
		{"/etc/xdg/", "/etc/xdg/"},
		{"/etc/", "/etc/"},
		{"~/.config/", "~/.config/"},
		{"~/.cache/", "~/.cache/"},
		{"~/Library/Application Support/", "~/Library/Application Support/"},
	}

	for _, repl := range replacements {
		if strings.HasPrefix(cleaned, repl.prefix) {
			return cleaned
		}
	}

	if strings.HasPrefix(cleaned, "~/.ssh/") {
		return "~/.ssh/***"
	}
	if strings.HasPrefix(cleaned, "~/.gnupg/") {
		return "~/.gnupg/***"
	}
	if strings.HasPrefix(cleaned, "~/.aws/") {
		return "~/.aws/***"
	}
	if strings.HasPrefix(cleaned, "~/.kube/") {
		return "~/.kube/***"
	}
	if strings.HasPrefix(cleaned, "~/.docker/") {
		return "~/.docker/***"
	}
	if strings.HasPrefix(cleaned, "~/.config/") {
		return cleaned
	}
	if strings.HasPrefix(cleaned, "/etc/") {
		return cleaned
	}

	return cleaned
}

func redactPathStrict(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if cleaned == "." || cleaned == "" {
		return path
	}

	if strings.HasPrefix(cleaned, "~/") {
		cleaned = strings.TrimPrefix(cleaned, "~/")
	}
	if strings.HasPrefix(cleaned, "/") {
		cleaned = strings.TrimPrefix(cleaned, "/")
	}

	parts := strings.Split(cleaned, "/")
	if len(parts) == 1 {
		return parts[0]
	}
	if len(parts) == 2 {
		return filepath.ToSlash(filepath.Join(parts[0], parts[1]))
	}

	top := parts[0]
	file := parts[len(parts)-1]
	if top == "home" && len(parts) >= 3 {
		top = parts[2]
	}
	if top == "Users" && len(parts) >= 3 {
		top = parts[2]
	}
	if top == "~" {
		top = "~"
	}
	return filepath.ToSlash(filepath.Join(top, file))
}
