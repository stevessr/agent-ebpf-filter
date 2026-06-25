package app

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/types"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section plugins.go ----

// PluginKind enumerates the plugin types supported by the registry.
type PluginKind = types.PluginKind

const (
	PluginKindEBPF    = types.PluginKindEBPF    // user-authored eBPF program (built via online builder)
	PluginKindWebhook = types.PluginKindWebhook // forwards selected events to an HTTP endpoint
	PluginKindCommand = types.PluginKindCommand // wrapper rewrite rule expressed as a plugin
)

// PluginAttachKind describes how an eBPF plugin attaches to the kernel.
type PluginAttachKind = types.PluginAttachKind

const (
	PluginAttachTracepoint = types.PluginAttachTracepoint
	PluginAttachKprobe     = types.PluginAttachKprobe
	PluginAttachKretprobe  = types.PluginAttachKretprobe
	PluginAttachLSM        = types.PluginAttachLSM
	PluginAttachNone       = types.PluginAttachNone
)

// PluginManifest is the on-disk descriptor for a registered plugin.
type PluginManifest = types.PluginManifest

var pluginIDRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

type pluginStore struct {
	mu      sync.RWMutex
	entries map[string]*PluginManifest
	loaded  bool
}

var pluginRegistry = &pluginStore{entries: make(map[string]*PluginManifest)}






func validatePluginID(id string) error {
	if !pluginIDRegex.MatchString(id) {
		return errors.New("plugin id must match [a-z0-9][a-z0-9-]{1,62}")
	}
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *pluginStore) ensureLoaded() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}
	if err := platform.MkdirAllAsRealUser(platform.PluginsRootDir(), 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(platform.PluginsRootDir())
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		path := platform.PluginManifestPath(id)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var manifest PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}
		if manifest.ID == "" {
			manifest.ID = id
		}
		s.entries[manifest.ID] = &manifest
	}
	s.loaded = true
	return nil
}

func (s *pluginStore) List() []PluginManifest {
	_ = s.ensureLoaded()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PluginManifest, 0, len(s.entries))
	for _, m := range s.entries {
		copy := *m
		copy.Loaded, copy.LoadError = ebpfPluginRuntimeState(copy.ID)
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *pluginStore) Get(id string) (PluginManifest, bool) {
	_ = s.ensureLoaded()
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.entries[id]
	if !ok {
		return PluginManifest{}, false
	}
	copy := *m
	copy.Loaded, copy.LoadError = ebpfPluginRuntimeState(copy.ID)
	return copy, true
}

func (s *pluginStore) saveLocked(m *PluginManifest) error {
	if err := platform.MkdirAllAsRealUser(platform.PluginDir(m.ID), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return platform.WriteFileAsRealUser(platform.PluginManifestPath(m.ID), data, 0644)
}

func (s *pluginStore) Upsert(m *PluginManifest) error {
	if err := validatePluginID(m.ID); err != nil {
		return err
	}
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.entries[m.ID]; ok {
		m.CreatedAt = existing.CreatedAt
	} else {
		m.CreatedAt = time.Now().UTC()
	}
	m.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(m); err != nil {
		return err
	}
	s.entries[m.ID] = m
	return nil
}

func (s *pluginStore) Delete(id string) error {
	_ = s.ensureLoaded()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[id]; !ok {
		return fmt.Errorf("plugin %q not found", id)
	}
	delete(s.entries, id)
	return os.RemoveAll(platform.PluginDir(id))
}

func (s *pluginStore) SetEnabled(id string, enabled bool) (PluginManifest, error) {
	_ = s.ensureLoaded()
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.entries[id]
	if !ok {
		return PluginManifest{}, fmt.Errorf("plugin %q not found", id)
	}
	m.Enabled = enabled
	m.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(m); err != nil {
		return PluginManifest{}, err
	}
	return *m, nil
}

// PluginSource returns the on-disk C source for an eBPF plugin (empty if absent).
func PluginSource(id string) (string, error) {
	if err := validatePluginID(id); err != nil {
		return "", err
	}
	data, err := os.ReadFile(platform.PluginSourcePath(id))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}


// PluginExportSummary is used when exporting/importing plugin manifests.
func pluginExportPayload() map[string]any {
	plugins := pluginRegistry.List()
	stripped := make([]PluginManifest, 0, len(plugins))
	for _, p := range plugins {
		p.Loaded = false
		p.LoadError = ""
		stripped = append(stripped, p)
	}
	return map[string]any{
		"plugins":    stripped,
		"exportedAt": time.Now().UTC(),
	}
}

func sanitizePluginName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "untitled-plugin"
	}
	return name
}
