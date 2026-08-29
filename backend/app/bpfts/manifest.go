package bpfts

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
)

const ManifestVersion = 1

var cIdentifierRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Manifest struct {
	Version int             `json:"version"`
	Source  string          `json:"source"`
	Probes  []ManifestProbe `json:"probes"`
	Maps    []ManifestMap   `json:"maps"`
}

type ManifestProbe struct {
	Name     string `json:"name"`
	Section  string `json:"section"`
	Kind     string `json:"kind"`
	Target   string `json:"target,omitempty"`
	Category string `json:"category,omitempty"`
	Event    string `json:"event,omitempty"`
}

type ManifestMap struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	MaxEntries uint32 `json:"maxEntries"`
}

func ParseManifest(r io.Reader) (Manifest, error) {
	var manifest Manifest
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode bpf-ts manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Manifest{}, fmt.Errorf("decode bpf-ts manifest: trailing JSON values are not allowed")
		}
		return Manifest{}, fmt.Errorf("decode bpf-ts manifest trailing data: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func LoadManifest(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("open bpf-ts manifest %q: %w", path, err)
	}
	defer file.Close()
	return ParseManifest(file)
}

func (manifest Manifest) Validate() error {
	if manifest.Version != ManifestVersion {
		return fmt.Errorf("unsupported bpf-ts manifest version %d (expected %d)", manifest.Version, ManifestVersion)
	}
	if manifest.Source == "" {
		return fmt.Errorf("bpf-ts manifest source must not be empty")
	}
	if len(manifest.Probes) == 0 {
		return fmt.Errorf("bpf-ts manifest must contain at least one probe")
	}

	probeNames := make(map[string]struct{}, len(manifest.Probes))
	for _, probe := range manifest.Probes {
		if !cIdentifierRE.MatchString(probe.Name) {
			return fmt.Errorf("invalid bpf-ts probe name %q", probe.Name)
		}
		if _, exists := probeNames[probe.Name]; exists {
			return fmt.Errorf("duplicate bpf-ts probe %q", probe.Name)
		}
		probeNames[probe.Name] = struct{}{}

		expectedSection := ""
		switch probe.Kind {
		case "kprobe":
			if probe.Target == "" || probe.Category != "" || probe.Event != "" {
				return fmt.Errorf("kprobe %q requires target only", probe.Name)
			}
			expectedSection = "kprobe/" + probe.Target
		case "uprobe":
			if probe.Target == "" || probe.Category != "" || probe.Event != "" {
				return fmt.Errorf("uprobe %q requires target only", probe.Name)
			}
			expectedSection = "uprobe"
		case "tracepoint":
			if probe.Category == "" || probe.Event == "" || probe.Target != "" {
				return fmt.Errorf("tracepoint %q requires category and event only", probe.Name)
			}
			expectedSection = "tracepoint/" + probe.Category + "/" + probe.Event
		default:
			return fmt.Errorf("probe %q has unsupported kind %q", probe.Name, probe.Kind)
		}
		if probe.Section != expectedSection {
			return fmt.Errorf("probe %q section mismatch: got %q, expected %q", probe.Name, probe.Section, expectedSection)
		}
	}

	mapNames := make(map[string]struct{}, len(manifest.Maps))
	for _, item := range manifest.Maps {
		if !cIdentifierRE.MatchString(item.Name) {
			return fmt.Errorf("invalid bpf-ts map name %q", item.Name)
		}
		if _, exists := mapNames[item.Name]; exists {
			return fmt.Errorf("duplicate bpf-ts map %q", item.Name)
		}
		mapNames[item.Name] = struct{}{}
		if _, conflicts := probeNames[item.Name]; conflicts {
			return fmt.Errorf("bpf-ts map %q conflicts with a probe name", item.Name)
		}
		switch item.Kind {
		case "ringbuf":
			if item.MaxEntries < 4096 || item.MaxEntries&(item.MaxEntries-1) != 0 {
				return fmt.Errorf("ringbuf %q capacity must be a power of two and at least 4096", item.Name)
			}
		case "hash", "array":
			if item.MaxEntries == 0 {
				return fmt.Errorf("map %q maxEntries must be positive", item.Name)
			}
		default:
			return fmt.Errorf("map %q has unsupported kind %q", item.Name, item.Kind)
		}
	}
	return nil
}
