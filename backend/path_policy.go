package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PathClass represents a semantic class for a filesystem path.
type PathClass string

const (
	PathClassWorkspace      PathClass = "workspace"
	PathClassSecret         PathClass = "secret"
	PathClassSystem         PathClass = "system"
	PathClassTemp           PathClass = "temp"
	PathClassBuildCache     PathClass = "build-cache"
	PathClassCredentialStore PathClass = "credential-store"
	PathClassUnknown        PathClass = "unknown"
)

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

// classifyPath determines the PathClass for a given absolute path.
// If cwd is provided, relative paths are resolved against it.
func classifyPath(path, cwd string) PathClass {
	normalized := normalizePathForClass(path, cwd)
	if normalized == "" {
		return PathClassUnknown
	}

	lower := strings.ToLower(normalized)

	// Check credential stores first (most sensitive)
	for _, rule := range pathClassRules {
		if rule.Class == PathClassCredentialStore {
			for _, pattern := range rule.Paths {
				if matchPathClassPattern(lower, normalizeHomePath(strings.ToLower(pattern))) {
					return PathClassCredentialStore
				}
			}
		}
	}

	// Check secrets
	for _, rule := range pathClassRules {
		if rule.Class == PathClassSecret {
			for _, pattern := range rule.Paths {
				if matchPathClassPattern(lower, normalizeHomePath(strings.ToLower(pattern))) {
					return PathClassSecret
				}
			}
		}
	}

	// Check system paths
	for _, rule := range pathClassRules {
		if rule.Class == PathClassSystem {
			for _, pattern := range rule.Paths {
				if matchPathClassPattern(lower, strings.ToLower(pattern)) {
					return PathClassSystem
				}
			}
		}
	}

	// Check temp
	for _, pattern := range pathClassRules[2].Paths {
		if matchPathClassPattern(lower, strings.ToLower(pattern)) {
			return PathClassTemp
		}
	}

	// Check build cache
	for _, rule := range pathClassRules {
		if rule.Class == PathClassBuildCache {
			for _, pattern := range rule.Paths {
				if matchPathClassPattern(lower, strings.ToLower(pattern)) {
					return PathClassBuildCache
				}
			}
		}
	}

	// Check workspace (the most common case)
	if cwd != "" {
		cleanCwd := filepath.Clean(cwd)
		if pathWithinBase(normalized, cleanCwd) {
			return PathClassWorkspace
		}
	}

	return PathClassUnknown
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

func normalizeHomePath(pattern string) string {
	if strings.HasPrefix(pattern, "~/") {
		return filepath.Join(homeDir(), pattern[2:])
	}
	return pattern
}

func homeDir() string {
	if sudoUserHomeCache != "" {
		return sudoUserHomeCache
	}
	return "/home"
}

func matchPathClassPattern(normalized, pattern string) bool {
	if strings.HasSuffix(pattern, "/") {
		// Directory prefix match
		return strings.HasPrefix(normalized, pattern) ||
			strings.HasPrefix(normalized, strings.TrimSuffix(pattern, "/"))
	}
	// Exact file match or path prefix match
	return normalized == pattern ||
		strings.HasPrefix(normalized, pattern+"/") ||
		strings.HasPrefix(normalized, pattern)
}

// pathClassTag maps a PathClass to the corresponding eBPF tag.
func pathClassTag(class PathClass) string {
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

// pathClassRisk maps a PathClass to a default risk score.
func pathClassRisk(class PathClass) float64 {
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

// classifyEventPath determines the PathClass for the path(s) in a kernel event.
func classifyBpfEventPath(event bpfEvent) PathClass {
	path := sanitizeUTF8(event.Path[:])
	if path != "" {
		return classifyPath(path, "")
	}
	extraPath := sanitizeUTF8(event.Extra4[:])
	if extraPath != "" {
		return classifyPath(extraPath, "")
	}
	return PathClassUnknown
}

// buildBpfEventPathClassSummary returns a human-readable summary of the event's path class.
func buildBpfEventPathClassSummary(event bpfEvent) string {
	class := classifyBpfEventPath(event)
	tag := pathClassTag(class)
	risk := pathClassRisk(class)
	return fmt.Sprintf("class=%s tag=%q risk=%.2f", class, tag, risk)
}
