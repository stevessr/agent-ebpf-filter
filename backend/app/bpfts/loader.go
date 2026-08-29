package bpfts

import (
	"errors"
	"fmt"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

type UprobeTarget struct {
	Path    string
	Symbol  string
	Address uint64
	Offset  uint64
	PID     int
}

type UprobeResolver func(ManifestProbe) (UprobeTarget, error)

type LoadOptions struct {
	Collection    ebpf.CollectionOptions
	ResolveUprobe UprobeResolver
}

type runtimeLink interface {
	Close() error
}

type probeAttachFunc func(ManifestProbe, *ebpf.Program, LoadOptions) (runtimeLink, error)

type Runtime struct {
	Collection *ebpf.Collection
	links      []runtimeLink
}

func closeLinksReverse(links []runtimeLink) error {
	var errs []error
	for index := len(links) - 1; index >= 0; index-- {
		if links[index] != nil {
			errs = append(errs, links[index].Close())
		}
	}
	return errors.Join(errs...)
}

func (runtime *Runtime) Close() error {
	if runtime == nil {
		return nil
	}
	closeErr := closeLinksReverse(runtime.links)
	runtime.links = nil
	if runtime.Collection != nil {
		runtime.Collection.Close()
		runtime.Collection = nil
	}
	return closeErr
}

func validateCollectionSpec(spec *ebpf.CollectionSpec, manifest Manifest) error {
	if spec == nil {
		return fmt.Errorf("nil eBPF collection spec")
	}
	manifestPrograms := make(map[string]struct{}, len(manifest.Probes))
	for _, probe := range manifest.Probes {
		manifestPrograms[probe.Name] = struct{}{}
		program, ok := spec.Programs[probe.Name]
		if !ok {
			return fmt.Errorf("manifest probe %q is missing from eBPF object", probe.Name)
		}
		if program.SectionName != probe.Section {
			return fmt.Errorf("program %q section mismatch: object has %q, manifest has %q", probe.Name, program.SectionName, probe.Section)
		}
	}
	for name := range spec.Programs {
		if _, ok := manifestPrograms[name]; !ok {
			return fmt.Errorf("eBPF object contains undeclared program %q", name)
		}
	}
	manifestMaps := make(map[string]struct{}, len(manifest.Maps))
	for _, item := range manifest.Maps {
		manifestMaps[item.Name] = struct{}{}
		if _, ok := spec.Maps[item.Name]; !ok {
			return fmt.Errorf("manifest map %q is missing from eBPF object", item.Name)
		}
	}
	for name := range spec.Maps {
		if _, ok := manifestMaps[name]; !ok {
			return fmt.Errorf("eBPF object contains undeclared map %q", name)
		}
	}
	return nil
}

func loadValidatedSpec(objectPath string, manifest Manifest) (*ebpf.CollectionSpec, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	spec, err := ebpf.LoadCollectionSpec(objectPath)
	if err != nil {
		return nil, fmt.Errorf("load bpf-ts object spec %q: %w", objectPath, err)
	}
	if err := validateCollectionSpec(spec, manifest); err != nil {
		return nil, fmt.Errorf("validate bpf-ts object against manifest: %w", err)
	}
	return spec, nil
}

// ValidateObjectManifest checks the ELF metadata contract without loading any
// BPF program into the kernel. It is safe to use from unprivileged CI jobs.
func ValidateObjectManifest(objectPath string, manifest Manifest) error {
	_, err := loadValidatedSpec(objectPath, manifest)
	return err
}

func needsUprobeResolver(kind string) bool {
	return kind == "uprobe" || kind == "uretprobe"
}

func attachUserProbe(probe ManifestProbe, program *ebpf.Program, resolver UprobeResolver) (runtimeLink, error) {
	if resolver == nil {
		return nil, fmt.Errorf("%s %q requires a UprobeResolver", probe.Kind, probe.Name)
	}
	target, err := resolver(probe)
	if err != nil {
		return nil, fmt.Errorf("resolve %s %q: %w", probe.Kind, probe.Name, err)
	}
	if target.Path == "" {
		return nil, fmt.Errorf("resolve %s %q: executable path is empty", probe.Kind, probe.Name)
	}
	executable, err := link.OpenExecutable(target.Path)
	if err != nil {
		return nil, fmt.Errorf("open executable for %s %q: %w", probe.Kind, probe.Name, err)
	}
	symbol := target.Symbol
	if symbol == "" && target.Address == 0 {
		symbol = probe.Target
	}
	options := &link.UprobeOptions{Address: target.Address, Offset: target.Offset, PID: target.PID}
	if probe.Kind == "uretprobe" {
		return executable.Uretprobe(symbol, program, options)
	}
	return executable.Uprobe(symbol, program, options)
}

func attachOneProbe(probe ManifestProbe, program *ebpf.Program, options LoadOptions) (runtimeLink, error) {
	switch probe.Kind {
	case "kprobe":
		return link.Kprobe(probe.Target, program, nil)
	case "kretprobe":
		return link.Kretprobe(probe.Target, program, nil)
	case "tracepoint":
		return link.Tracepoint(probe.Category, probe.Event, program, nil)
	case "uprobe", "uretprobe":
		return attachUserProbe(probe, program, options.ResolveUprobe)
	default:
		return nil, fmt.Errorf("unsupported probe kind %q", probe.Kind)
	}
}

func attachProbeSet(programs map[string]*ebpf.Program, manifest Manifest, options LoadOptions, attach probeAttachFunc) ([]runtimeLink, error) {
	if attach == nil {
		attach = attachOneProbe
	}
	attached := make([]runtimeLink, 0, len(manifest.Probes))
	cleanup := func(cause error) ([]runtimeLink, error) {
		closeErr := closeLinksReverse(attached)
		if closeErr != nil {
			return nil, errors.Join(cause, fmt.Errorf("cleanup attached bpf-ts probes: %w", closeErr))
		}
		return nil, cause
	}
	for _, probe := range manifest.Probes {
		program := programs[probe.Name]
		if program == nil {
			return cleanup(fmt.Errorf("loaded collection is missing program %q", probe.Name))
		}
		attachedLink, err := attach(probe, program, options)
		if err != nil {
			return cleanup(fmt.Errorf("attach %s probe %q: %w", probe.Kind, probe.Name, err))
		}
		if attachedLink == nil {
			return cleanup(fmt.Errorf("attach %s probe %q returned a nil link", probe.Kind, probe.Name))
		}
		attached = append(attached, attachedLink)
	}
	return attached, nil
}

func LoadAndAttach(objectPath string, manifest Manifest, options LoadOptions) (*Runtime, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	for _, probe := range manifest.Probes {
		if needsUprobeResolver(probe.Kind) && options.ResolveUprobe == nil {
			return nil, fmt.Errorf("%s %q requires a UprobeResolver", probe.Kind, probe.Name)
		}
	}
	spec, err := loadValidatedSpec(objectPath, manifest)
	if err != nil {
		return nil, err
	}
	collection, err := ebpf.NewCollectionWithOptions(spec, options.Collection)
	if err != nil {
		return nil, fmt.Errorf("load bpf-ts collection: %w", err)
	}
	links, err := attachProbeSet(collection.Programs, manifest, options, attachOneProbe)
	if err != nil {
		collection.Close()
		return nil, err
	}
	return &Runtime{Collection: collection, links: links}, nil
}
