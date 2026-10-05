package app

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"agent-ebpf-filter/pb"
)

const ignoredPathBypassRiskScore = 60

func defaultIgnoredEventPaths() []string {
	return []string{
		"/proc",
		"/sys/bus",
		"/sys/class",
		"/sys/devices",
		"/sys/fs/cgroup",
		"/sys/kernel/mm",
		"/dev/null",
		"/dev/random",
		"/dev/urandom",
		"/dev/zero",
		"/etc/ld.so.cache",
		"/etc/localtime",
		"/usr/lib/locale",
		"/usr/share/locale",
		"/usr/share/zoneinfo",
	}
}

func normalizeIgnoredEventPaths(values []string) ([]string, error) {
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

func pathMatchesIgnoredPrefix(path string, ignored []string) bool {
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

func eventPathNoiseEligible(event *pb.Event) bool {
	if event == nil {
		return false
	}
	switch event.GetEventType() {
	case pb.EventType_OPENAT, pb.EventType_OPEN, pb.EventType_READ:
		return true
	}

	switch strings.ToLower(strings.TrimSpace(event.GetType())) {
	case "openat", "open", "read",
		"stat", "lstat", "fstat", "newfstatat", "statx",
		"access", "faccessat", "faccessat2",
		"readlink", "readlinkat", "getdents", "getdents64":
		return true
	default:
		return false
	}
}

func eventBypassesIgnoredPaths(event *pb.Event) bool {
	if event == nil {
		return false
	}
	decision := strings.ToUpper(strings.TrimSpace(event.GetDecision()))
	if strings.Contains(decision, "BLOCK") ||
		strings.Contains(decision, "DENY") ||
		strings.Contains(decision, "ALERT") {
		return true
	}
	if event.GetRiskScore() >= ignoredPathBypassRiskScore {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(event.GetType())) {
	case "semantic_alert", "agentsight_alert":
		return true
	default:
		return false
	}
}

// shouldIgnoreEventPath only suppresses low-risk read/open/metadata telemetry.
// Mutating, executable, permission-changing and ioctl events are never eligible.
// Semantic analysis runs before this gate, and alert/block/high-risk events
// always bypass it.
func shouldIgnoreEventPath(event *pb.Event) bool {
	if event == nil ||
		!eventPathNoiseEligible(event) ||
		eventBypassesIgnoredPaths(event) ||
		runtimeSettingsStore == nil {
		return false
	}

	runtimeSettingsStore.mu.RLock()
	ignored := runtimeSettingsStore.settings.IgnoredPaths
	matched := pathMatchesIgnoredPrefix(event.GetPath(), ignored) ||
		pathMatchesIgnoredPrefix(event.GetExtraPath(), ignored)
	runtimeSettingsStore.mu.RUnlock()
	return matched
}
