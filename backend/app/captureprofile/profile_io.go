package captureprofile

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

func ParseJSON(data []byte) ([]Profile, error) {
	var profiles []Profile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("decode API capture profiles: %w", err)
	}
	validator := NewRegistry(nil)
	if err := validator.Replace(profiles); err != nil {
		return nil, err
	}
	return validator.Profiles(), nil
}

func LoadJSON(path string) ([]Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read API capture profiles: %w", err)
	}
	return ParseJSON(data)
}

// MergeProfiles overlays custom profiles by ID while retaining all built-ins
// that were not explicitly replaced. The result is validated atomically by the
// Registry before publication.
func MergeProfiles(base, overlay []Profile) []Profile {
	merged := make(map[string]Profile, len(base)+len(overlay))
	for _, profile := range base {
		profile = normalizeProfile(profile)
		if profile.ID != "" {
			merged[profile.ID] = profile
		}
	}
	for _, profile := range overlay {
		profile = normalizeProfile(profile)
		if profile.ID != "" {
			merged[profile.ID] = profile
		}
	}
	out := make([]Profile, 0, len(merged))
	for _, profile := range merged {
		out = append(out, profile)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func ReloadDefaultJSON(path string) error {
	overlay, err := LoadJSON(path)
	if err != nil {
		return err
	}
	return Default.Replace(MergeProfiles(BuiltinProfiles(), overlay))
}

// WatchDefaultJSON hashes the file contents instead of relying only on mtime,
// which makes atomic replace/ConfigMap style updates reliable. Invalid updates
// never replace the last known-good immutable registry snapshot.
func WatchDefaultJSON(ctx context.Context, path string, interval time.Duration, onError func(error)) {
	if ctx == nil || strings.TrimSpace(path) == "" {
		return
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	var lastAttempt [32]byte
	var attempted bool
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := os.ReadFile(path)
			if err != nil {
				if onError != nil {
					onError(fmt.Errorf("read API capture profiles: %w", err))
				}
				continue
			}
			digest := sha256.Sum256(data)
			if attempted && digest == lastAttempt {
				continue
			}
			lastAttempt = digest
			attempted = true
			overlay, err := ParseJSON(data)
			if err == nil {
				err = Default.Replace(MergeProfiles(BuiltinProfiles(), overlay))
			}
			if err != nil && onError != nil {
				onError(err)
			}
		}
	}
}
