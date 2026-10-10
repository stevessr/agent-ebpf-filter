// Package agentscope defines the pure, transport-independent rules for
// deciding whether an Agent's events enter capture and monitoring pipelines.
// It does not change or enforce kernel execution policies.
package agentscope

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	ModeBlacklist = "blacklist"
	ModeWhitelist = "whitelist"
	MaxEntries    = 256
)

// List and Policy retain the existing JSON API shape and default semantics.
type List struct {
	Mode    string   `json:"mode"`
	Entries []string `json:"entries"`
}

type Policy struct {
	Capture List `json:"capture"`
	Monitor List `json:"monitor"`
}

func Default() Policy {
	return Policy{
		Capture: List{Mode: ModeBlacklist, Entries: []string{}},
		Monitor: List{Mode: ModeBlacklist, Entries: []string{}},
	}
}

// ValidateList canonicalizes names, preserving the original exact-match rules.
// In whitelist mode, an empty list rejects all; an empty blacklist allows all.
func ValidateList(list List) (List, error) {
	if list.Mode != ModeBlacklist && list.Mode != ModeWhitelist {
		return List{}, fmt.Errorf("mode must be blacklist or whitelist")
	}
	if len(list.Entries) > MaxEntries {
		return List{}, fmt.Errorf("too many entries (max %d)", MaxEntries)
	}
	out := List{Mode: list.Mode, Entries: make([]string, 0, len(list.Entries))}
	seen := make(map[string]struct{}, len(list.Entries))
	for _, raw := range list.Entries {
		entry := strings.ToLower(strings.TrimSpace(raw))
		if !utf8.ValidString(entry) || len(entry) == 0 || len(entry) > 128 ||
			strings.ContainsAny(entry, "\x00\r\n\t") {
			return List{}, errors.New("entries must be nonempty UTF-8 names (max 128 bytes)")
		}
		if _, found := seen[entry]; !found {
			seen[entry] = struct{}{}
			out.Entries = append(out.Entries, entry)
		}
	}
	return out, nil
}

func Validate(input Policy) (Policy, error) {
	capture, err := ValidateList(input.Capture)
	if err != nil {
		return Policy{}, fmt.Errorf("capture: %w", err)
	}
	monitor, err := ValidateList(input.Monitor)
	if err != nil {
		return Policy{}, fmt.Errorf("monitor: %w", err)
	}
	return Policy{Capture: capture, Monitor: monitor}, nil
}

// Allows matches exact process comm or tag (case insensitive). The caller is
// responsible for supplying an observed identity, not an inferred PID owner.
func Allows(list List, comm, tag string) bool {
	return AllowsWithOwner(list, comm, tag, "")
}

// AllowsWithOwner also tests a separately verified Agent root name. It must
// never guess the owner from the command of an unverified child process.
func AllowsWithOwner(list List, comm, tag, owner string) bool {
	comm = strings.TrimSpace(comm)
	tag = strings.TrimSpace(tag)
	owner = strings.TrimSpace(owner)
	matched := false
	for _, entry := range list.Entries {
		if strings.EqualFold(entry, comm) || strings.EqualFold(entry, tag) ||
			(owner != "" && strings.EqualFold(entry, owner)) {
			matched = true
			break
		}
	}
	if list.Mode == ModeWhitelist {
		return matched
	}
	return !matched
}
