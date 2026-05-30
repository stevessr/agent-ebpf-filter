package launchenv

import (
	"os"
	"sort"
	"strings"
)

type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var excludedExact = map[string]struct{}{
	"DISABLE_AUTH": {},
	"GIN_MODE":     {},
	"PKEXEC_UID":   {},
	"SUDO_UID":     {},
	"SUDO_GID":     {},
	"SUDO_USER":    {},
}

var excludedPrefixes = []string{
	"AGENT_",
}

func IsBackendRuntimeKey(key string) bool {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return true
	}
	if _, ok := excludedExact[trimmed]; ok {
		return true
	}
	for _, prefix := range excludedPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func Collect() []Entry {
	items := make([]Entry, 0, len(os.Environ()))
	for _, raw := range os.Environ() {
		key, value, ok := strings.Cut(raw, "=")
		if !ok || IsBackendRuntimeKey(key) {
			continue
		}
		items = append(items, Entry{
			Key:   key,
			Value: value,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Key == items[j].Key {
			return items[i].Value < items[j].Value
		}
		return strings.ToLower(items[i].Key) < strings.ToLower(items[j].Key)
	})
	return items
}
