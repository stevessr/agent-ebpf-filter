package app

import (
	"context"
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
	"unicode/utf8"
)

// ---- moved from backend/zz_merged_backend.go section plugins.go ----

// PluginKind, PluginAttachKind and PluginManifest are aliased from the
// types subpackage via typebridge.go — they are not re-defined here.

var pluginIDRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)
var pluginSHA256Regex = regexp.MustCompile(`^[a-f0-9]{64}$`)

const (
	maxRegisteredPlugins      = 256
	maxPluginNameBytes        = 256
	maxPluginDescriptionBytes = 8 << 10
	maxPluginMetadataBytes    = 1024
	maxPluginURLBytes         = 4 << 10
	maxPluginListItems        = 128
	maxPluginListItemBytes    = 4 << 10
)

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

func clonePluginManifest(manifest *PluginManifest) *PluginManifest {
	if manifest == nil {
		return nil
	}
	clone := *manifest
	clone.WebhookEvents = append([]string(nil), manifest.WebhookEvents...)
	clone.CommandArgs = append([]string(nil), manifest.CommandArgs...)
	clone.CommandRewrite = append([]string(nil), manifest.CommandRewrite...)
	return &clone
}

func validatePluginManifest(manifest *PluginManifest) error {
	if manifest == nil {
		return errors.New("plugin manifest is nil")
	}
	if err := validatePluginID(manifest.ID); err != nil {
		return err
	}
	for label, field := range map[string]struct {
		value string
		max   int
	}{
		"name":        {manifest.Name, maxPluginNameBytes},
		"description": {manifest.Description, maxPluginDescriptionBytes},
		"author":      {manifest.Author, maxPluginMetadataBytes},
		"version":     {manifest.Version, maxPluginMetadataBytes},
		"webhookUrl":  {manifest.WebhookURL, maxPluginURLBytes},
		"commandComm": {manifest.CommandComm, maxPluginMetadataBytes},
		"commandRule": {manifest.CommandRule, maxPluginMetadataBytes},
	} {
		if !utf8.ValidString(field.value) || strings.IndexByte(field.value, 0) >= 0 || len(field.value) > field.max {
			return fmt.Errorf("plugin %s is invalid or exceeds %d bytes", label, field.max)
		}
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return errors.New("plugin name is required")
	}
	for label, digest := range map[string]string{
		"sourceSha256": manifest.SourceSHA256,
		"objectSha256": manifest.ObjectSHA256,
	} {
		if digest != "" && !pluginSHA256Regex.MatchString(digest) {
			return fmt.Errorf("plugin %s must be a lowercase SHA-256 digest", label)
		}
	}
	for label, values := range map[string][]string{
		"webhookEvents":  manifest.WebhookEvents,
		"commandArgs":    manifest.CommandArgs,
		"commandRewrite": manifest.CommandRewrite,
	} {
		if len(values) > maxPluginListItems {
			return fmt.Errorf("plugin %s exceeds %d items", label, maxPluginListItems)
		}
		for _, value := range values {
			if !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 || len(value) > maxPluginListItemBytes {
				return fmt.Errorf("plugin %s contains an invalid item", label)
			}
		}
	}

	switch manifest.Kind {
	case PluginKindEBPF:
		switch manifest.AttachKind {
		case "", PluginAttachNone:
			if manifest.Enabled {
				return errors.New("enabled eBPF plugin requires an attach kind")
			}
		case PluginAttachTracepoint, PluginAttachKprobe, PluginAttachKretprobe, PluginAttachLSM:
			if err := validateLoadableEBPFPluginManifest(manifest); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported attach kind %q", manifest.AttachKind)
		}
	case PluginKindWebhook, PluginKindCommand:
		// Runtime-specific fields are bounded above; their executors perform
		// additional semantic validation when those plugin kinds are enabled.
	default:
		return fmt.Errorf("unsupported plugin kind %q", manifest.Kind)
	}
	return nil
}

func (s *pluginStore) ensureLoaded() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}
	root, err := openPluginsRoot()
	if err != nil {
		return err
	}
	defer root.Close()
	names, err := researchDirectoryNames(root, 1024)
	if err != nil {
		return err
	}
	for _, id := range names {
		if err := validatePluginID(id); err != nil {
			continue
		}
		data, err := readPluginFile(id, "manifest.json", pluginManifestMaxBytes)
		if err != nil {
			continue
		}
		var manifest PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}
		if manifest.ID != id {
			continue
		}
		if err := validatePluginManifest(&manifest); err != nil {
			continue
		}
		manifest.Loaded = false
		manifest.LoadError = ""
		s.entries[id] = clonePluginManifest(&manifest)
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
		copy := *clonePluginManifest(m)
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
	copy := *clonePluginManifest(m)
	copy.Loaded, copy.LoadError = ebpfPluginRuntimeState(copy.ID)
	return copy, true
}

func (s *pluginStore) saveLocked(m *PluginManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writePluginFile(m.ID, "manifest.json", data, pluginManifestMaxBytes)
}

func (s *pluginStore) Upsert(m *PluginManifest) error {
	return s.upsert(context.Background(), m, nil)
}

func (s *pluginStore) UpsertWithSource(m *PluginManifest, source string) error {
	return s.upsert(context.Background(), m, &source)
}

func (s *pluginStore) UpsertWithSourceContext(ctx context.Context, m *PluginManifest, source string) error {
	return s.upsert(ctx, m, &source)
}

func (s *pluginStore) upsert(ctx context.Context, m *PluginManifest, source *string) error {
	if m == nil {
		return errors.New("plugin manifest is nil")
	}
	if err := validatePluginID(m.ID); err != nil {
		return err
	}
	if source != nil {
		if m.Kind != PluginKindEBPF {
			return errors.New("source is only supported for eBPF plugins")
		}
		if len(*source) > int(pluginSourceMaxBytes) {
			return fmt.Errorf("plugin source exceeds %d bytes", pluginSourceMaxBytes)
		}
	}
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	releaseArtifacts, err := acquirePluginArtifactLock(ctx, m.ID)
	if err != nil {
		return err
	}
	defer releaseArtifacts()
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clonePluginManifest(m)
	next.Loaded = false
	next.LoadError = ""

	var existing *PluginManifest
	if current, ok := s.entries[m.ID]; ok {
		next.CreatedAt = current.CreatedAt
		existing = clonePluginManifest(current)
	} else {
		if len(s.entries) >= maxRegisteredPlugins {
			return fmt.Errorf("registered plugin limit (%d) reached", maxRegisteredPlugins)
		}
		next.CreatedAt = time.Now().UTC()
	}
	next.UpdatedAt = time.Now().UTC()

	var oldSource []byte
	oldSourceExists := false
	if source != nil {
		var err error
		oldSource, err = readPluginFile(next.ID, "source.c", pluginSourceMaxBytes)
		if err == nil {
			oldSourceExists = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		next.SourceSHA256 = sha256Hex([]byte(*source))
		next.ObjectSHA256 = ""
		if existing != nil && existing.Kind == PluginKindEBPF && existing.SourceSHA256 == next.SourceSHA256 {
			next.ObjectSHA256 = existing.ObjectSHA256
		}
		if oldSourceExists && string(oldSource) == *source {
			object, err := readPluginFile(next.ID, "program.o", maxUserBPFObjectBytes)
			if err == nil {
				next.ObjectSHA256 = sha256Hex(object)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	} else if existing != nil && existing.Kind == next.Kind {
		next.SourceSHA256 = existing.SourceSHA256
		next.ObjectSHA256 = existing.ObjectSHA256
	}
	if next.Kind == PluginKindEBPF && next.Enabled && next.ObjectSHA256 == "" {
		return errors.New("enabled eBPF plugin requires a compiled object checksum")
	}
	if err := validatePluginManifest(next); err != nil {
		return err
	}

	rollbackSource := func() error {
		if source == nil {
			return nil
		}
		if oldSourceExists {
			return writePluginFile(next.ID, "source.c", oldSource, pluginSourceMaxBytes)
		}
		return removePluginFileIfExists(next.ID, "source.c")
	}
	if source != nil {
		if err := writePluginSource(next.ID, *source); err != nil {
			return err
		}
		if next.ObjectSHA256 == "" {
			if err := removePluginFileIfExists(next.ID, "program.o"); err != nil {
				return errors.Join(err, rollbackSource())
			}
		}
	}
	if err := s.saveLocked(next); err != nil {
		return errors.Join(err, rollbackSource())
	}
	s.entries[next.ID] = next
	return nil
}

func (s *pluginStore) RecordCompile(id, sourceSHA256, objectSHA256 string) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.entries[id]
	if !ok {
		return nil
	}
	if existing.Kind != PluginKindEBPF {
		return errors.New("not an eBPF plugin")
	}
	if existing.SourceSHA256 != "" && existing.SourceSHA256 != sourceSHA256 {
		return errors.New("plugin source changed while compilation was running")
	}
	next := clonePluginManifest(existing)
	next.SourceSHA256 = sourceSHA256
	next.ObjectSHA256 = objectSHA256
	next.UpdatedAt = time.Now().UTC()
	if err := validatePluginManifest(next); err != nil {
		return err
	}
	if err := s.saveLocked(next); err != nil {
		return err
	}
	s.entries[id] = next
	return nil
}

func (s *pluginStore) Delete(id string) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	releaseArtifacts, err := acquirePluginArtifactLock(context.Background(), id)
	if err != nil {
		return err
	}
	defer releaseArtifacts()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[id]; !ok {
		return fmt.Errorf("plugin %q not found", id)
	}
	// Cancel both pre-existing loads and any load that raced with file
	// deletion. A post-delete load can no longer open the object.
	UnloadEBPFPlugin(id)
	if err := deletePluginFiles(id); err != nil {
		return err
	}
	UnloadEBPFPlugin(id)
	delete(s.entries, id)
	return nil
}

func (s *pluginStore) SetEnabled(id string, enabled bool) (PluginManifest, error) {
	_ = s.ensureLoaded()
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.entries[id]
	if !ok {
		return PluginManifest{}, fmt.Errorf("plugin %q not found", id)
	}
	next := clonePluginManifest(m)
	next.Enabled = enabled
	next.UpdatedAt = time.Now().UTC()
	if next.Kind == PluginKindEBPF && enabled && next.ObjectSHA256 == "" {
		return PluginManifest{}, errors.New("enabled eBPF plugin requires a compiled object checksum")
	}
	if err := validatePluginManifest(next); err != nil {
		return PluginManifest{}, err
	}
	if err := s.saveLocked(next); err != nil {
		return PluginManifest{}, err
	}
	s.entries[id] = next
	return *clonePluginManifest(next), nil
}

// PluginSource returns the on-disk C source for an eBPF plugin (empty if absent).
func PluginSource(id string) (string, error) {
	if err := validatePluginID(id); err != nil {
		return "", err
	}
	data, err := readPluginFile(id, "source.c", pluginSourceMaxBytes)
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
