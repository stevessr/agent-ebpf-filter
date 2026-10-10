// Package eventnoise implements pure, transport-agnostic low-risk event
// suppression. It must never suppress a policy decision or high-risk alert.
package eventnoise

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

const BypassRiskScore int64 = 60

type Event struct {
	Type            string
	Decision        string
	Path            string
	ExtraPath       string
	RiskScore       int64
	KernelOpenRead  bool
	KernelReadWrite bool
}

func DefaultIgnoredPaths() []string {
	return []string{"/proc", "/tmp"}
}

// RoutineNoisePaths applies only to routine read/open/metadata operations and
// only if the user has enabled at least one ignored path.
func RoutineNoisePaths() []string {
	return []string{
		"/sys/bus", "/sys/class", "/sys/devices", "/sys/fs/cgroup",
		"/sys/kernel/mm", "/dev/null", "/dev/random", "/dev/urandom",
		"/dev/zero", "/etc/ld.so.cache", "/etc/localtime",
		"/usr/lib/locale", "/usr/share/locale", "/usr/share/zoneinfo",
	}
}

// NormalizeIgnoredPaths keeps nil distinct from an explicit empty list, so an
// explicit [] can disable all default suppressions in persisted settings.
func NormalizeIgnoredPaths(values []string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			return nil, errors.New("ignored event path must be absolute")
		}
		value = filepath.Clean(value)
		if value == string(filepath.Separator) {
			return nil, errors.New("ignored event path cannot be filesystem root")
		}
		if len(value) > 4096 {
			return nil, errors.New("ignored event path exceeds supported length")
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func MatchesPrefix(path string, ignored []string) bool {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	path = filepath.Clean(path)
	for _, prefix := range ignored {
		if path == prefix || strings.HasPrefix(path, prefix+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func ReadOrMetadataEligible(eventType string, kernelOpenRead bool) bool {
	if kernelOpenRead {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "openat", "open", "read",
		"stat", "lstat", "fstat", "newfstatat", "statx",
		"access", "faccessat", "faccessat2",
		"readlink", "readlinkat", "getdents", "getdents64":
		return true
	default:
		return false
	}
}

// Ordinary /usr/bin reads/writes are noise, but file mutations and execution
// must never be suppressed solely by the /usr/bin rule.
func UsrBinReadWriteNoise(eventType, path, extraPath string, kernelReadWrite bool) bool {
	if !kernelReadWrite {
		switch strings.ToLower(strings.TrimSpace(eventType)) {
		case "read", "write", "pread64", "pwrite64", "readv", "writev":
		default:
			return false
		}
	}
	return MatchesPrefix(path, []string{"/usr/bin"}) ||
		MatchesPrefix(extraPath, []string{"/usr/bin"})
}

func BypassesIgnoredPaths(decision string, riskScore int64, eventType string) bool {
	decision = strings.ToUpper(strings.TrimSpace(decision))
	if strings.Contains(decision, "BLOCK") ||
		strings.Contains(decision, "DENY") ||
		strings.Contains(decision, "ALERT") {
		return true
	}
	if riskScore >= BypassRiskScore {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "semantic_alert", "agentsight_alert":
		return true
	default:
		return false
	}
}

// ShouldIgnore applies only to presentation/archive admission, never kernel
// enforcement. The caller must already have performed semantic analysis.
func ShouldIgnore(event Event, configured []string) bool {
	if BypassesIgnoredPaths(event.Decision, event.RiskScore, event.Type) ||
		len(configured) == 0 {
		return false
	}
	if MatchesPrefix(event.Path, configured) || MatchesPrefix(event.ExtraPath, configured) ||
		UsrBinReadWriteNoise(event.Type, event.Path, event.ExtraPath, event.KernelReadWrite) {
		return true
	}
	if !ReadOrMetadataEligible(event.Type, event.KernelOpenRead) {
		return false
	}
	routine := RoutineNoisePaths()
	return MatchesPrefix(event.Path, routine) || MatchesPrefix(event.ExtraPath, routine)
}
