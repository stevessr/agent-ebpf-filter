package pathpolicy

import (
	"path/filepath"
	"strings"
)

type PathClass string

type pathClassRule struct {
	Class PathClass
	Paths []string
}

var pathClassRules = []pathClassRule{
	{Class: PathClassSecret, Paths: []string{
		"~/.ssh/",
		"~/.gnupg/",
		"~/.aws/",
		"~/.config/gcloud/",
		"~/.kube/",
		"~/.docker/config.json",
		"/etc/shadow",
		"/etc/ssl/private/",
	}},
	{Class: PathClassSystem, Paths: []string{
		"/etc/",
		"/boot/",
		"/sys/",
		"/proc/",
		"/dev/",
		"/lib/modules/",
		"/usr/lib/systemd/",
		"/var/log/",
	}},
	{Class: PathClassTemp, Paths: []string{
		"/tmp/",
		"/var/tmp/",
		"/dev/shm/",
		"/run/user/",
	}},
	{Class: PathClassBuildCache, Paths: []string{
		"target/",
		"node_modules/",
		".venv/",
		"venv/",
		"__pycache__/",
		".cache/",
		"dist/",
		"build/",
		".gradle/",
		".m2/",
		".cargo/",
	}},
	{Class: PathClassCredentialStore, Paths: []string{
		"~/.netrc",
		"~/.git-credentials",
		"~/.npmrc",
		"~/.pypirc",
		"~/.gem/credentials",
		"~/.cargo/credentials.toml",
		"~/.docker/config.json",
	}},
}

func Classify(path, cwd, home string) PathClass {
	normalized := normalizePathForClass(path, cwd)
	if normalized == "" {
		return PathClassUnknown
	}

	lower := strings.ToLower(normalized)

	for _, rule := range pathClassRules {
		if rule.Class == PathClassCredentialStore {
			for _, pattern := range rule.Paths {
				if matchPathClassPattern(lower, normalizeHomePath(strings.ToLower(pattern), home)) {
					return PathClassCredentialStore
				}
			}
		}
	}

	for _, rule := range pathClassRules {
		if rule.Class == PathClassSecret {
			for _, pattern := range rule.Paths {
				if matchPathClassPattern(lower, normalizeHomePath(strings.ToLower(pattern), home)) {
					return PathClassSecret
				}
			}
		}
	}

	for _, rule := range pathClassRules {
		if rule.Class == PathClassSystem {
			for _, pattern := range rule.Paths {
				if matchPathClassPattern(lower, strings.ToLower(pattern)) {
					return PathClassSystem
				}
			}
		}
	}

	for _, pattern := range pathClassRules[2].Paths {
		if matchPathClassPattern(lower, strings.ToLower(pattern)) {
			return PathClassTemp
		}
	}

	for _, rule := range pathClassRules {
		if rule.Class == PathClassBuildCache {
			for _, pattern := range rule.Paths {
				if matchPathClassPattern(lower, strings.ToLower(pattern)) {
					return PathClassBuildCache
				}
			}
		}
	}

	if cwd != "" {
		cleanCwd := filepath.Clean(cwd)
		if pathWithinBase(normalized, cleanCwd) {
			return PathClassWorkspace
		}
	}

	return PathClassUnknown
}

func Tag(class PathClass) string {
	switch class {
	case PathClassWorkspace:
		return "Workspace"
	case PathClassSecret:
		return "Secret"
	case PathClassSystem:
		return "System"
	case PathClassTemp:
		return "Temp"
	case PathClassBuildCache:
		return "Build Cache"
	case PathClassCredentialStore:
		return "Credential Store"
	default:
		return "Unknown"
	}
}

func Risk(class PathClass) float64 {
	switch class {
	case PathClassSecret:
		return 0.95
	case PathClassCredentialStore:
		return 0.98
	case PathClassSystem:
		return 0.85
	case PathClassWorkspace:
		return 0.05
	case PathClassBuildCache:
		return 0.10
	case PathClassTemp:
		return 0.20
	default:
		return 0.30
	}
}

func normalizePathForClass(path, cwd string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	cwd = strings.TrimSpace(cwd)

	if !filepath.IsAbs(path) && cwd != "" && filepath.IsAbs(cwd) {
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}

func normalizeHomePath(pattern, home string) string {
	if strings.HasPrefix(pattern, "~/") {
		if home == "" {
			home = "/home"
		}
		return filepath.Join(home, pattern[2:])
	}
	return pattern
}

func matchPathClassPattern(normalized, pattern string) bool {
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(normalized, pattern) ||
			strings.HasPrefix(normalized, strings.TrimSuffix(pattern, "/"))
	}
	return normalized == pattern ||
		strings.HasPrefix(normalized, pattern+"/") ||
		strings.HasPrefix(normalized, pattern)
}

func pathWithinBase(path, base string) bool {
	path = filepath.Clean(path)
	base = filepath.Clean(base)
	if path == base {
		return true
	}
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}
