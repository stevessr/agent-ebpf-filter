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

type Runtime struct {
	Collection *ebpf.Collection
	links      []link.Link
}

func (runtime *Runtime) Close() error {
	if runtime == nil {
		return nil
	}
	var errs []error
	for index := len(runtime.links) - 1; index >= 0; index-- {
		if runtime.links[index] != nil {
			errs = append(errs, runtime.links[index].Close())
		}
	}
	runtime.links = nil
	if runtime.Collection != nil {
		runtime.Collection.Close()
		runtime.Collection = nil
	}
	return errors.Join(errs...)
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
			return fmt.Errorf(
				"program %q section mismatch: object has %q, manifest has %q",
				probe.Name,
				program.SectionName,
				probe.Section,
			)
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

func LoadAndAttach(objectPath string, manifest Manifest, options LoadOptions) (*Runtime, error) {
	for _, probe := range manifest.Probes {
		if probe.Kind == "uprobe" && options.ResolveUprobe == nil {
			return nil, fmt.Errorf("uprobe %q requires a UprobeResolver", probe.Name)
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

	runtime := &Runtime{Collection: collection}
	cleanup := func(cause error) (*Runtime, error) {
		closeErr := runtime.Close()
		if closeErr != nil {
			return nil, errors.Join(cause, fmt.Errorf("cleanup bpf-ts runtime: %w", closeErr))
		}
		return nil, cause
	}

	for _, probe := range manifest.Probes {
		program := collection.Programs[probe.Name]
		if program == nil {
			return cleanup(fmt.Errorf("loaded collection is missing program %q", probe.Name))
		}

		var attached link.Link
		switch probe.Kind {
		case "kprobe":
			attached, err = link.Kprobe(probe.Target, program, nil)
		case "tracepoint":
			attached, err = link.Tracepoint(probe.Category, probe.Event, program, nil)
		case "uprobe":
			target, resolveErr := options.ResolveUprobe(probe)
			if resolveErr != nil {
				return cleanup(fmt.Errorf("resolve uprobe %q: %w", probe.Name, resolveErr))
			}
			if target.Path == "" {
				return cleanup(fmt.Errorf("resolve uprobe %q: executable path is empty", probe.Name))
			}
			executable, openErr := link.OpenExecutable(target.Path)
			if openErr != nil {
				return cleanup(fmt.Errorf("open executable for uprobe %q: %w", probe.Name, openErr))
			}
			symbol := target.Symbol
			if symbol == "" && target.Address == 0 {
				symbol = probe.Target
			}
			attached, err = executable.Uprobe(symbol, program, &link.UprobeOptions{
				Address: target.Address,
				Offset:  target.Offset,
				PID:     target.PID,
			})
		default:
			return cleanup(fmt.Errorf("unsupported probe kind %q", probe.Kind))
		}
		if err != nil {
			return cleanup(fmt.Errorf("attach %s probe %q: %w", probe.Kind, probe.Name, err))
		}
		runtime.links = append(runtime.links, attached)
	}

	return runtime, nil
}
