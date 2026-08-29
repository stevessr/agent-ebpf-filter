package main

import (
	"fmt"
	"os"

	"agent-ebpf-filter/app/bpfts"
	"github.com/cilium/ebpf/rlimit"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: bpfts-attach-check <manifest.json> <object.o> <executable-or-library>")
		os.Exit(2)
	}
	manifestPath := os.Args[1]
	objectPath := os.Args[2]
	targetPath := os.Args[3]

	manifest, err := bpfts.LoadManifest(manifestPath)
	if err != nil {
		fatal(err)
	}
	if err := rlimit.RemoveMemlock(); err != nil {
		fatal(fmt.Errorf("remove memlock limit: %w", err))
	}

	runtime, err := bpfts.LoadAndAttach(objectPath, manifest, bpfts.LoadOptions{
		ResolveUprobe: func(probe bpfts.ManifestProbe) (bpfts.UprobeTarget, error) {
			if probe.Kind != "uprobe" && probe.Kind != "uretprobe" {
				return bpfts.UprobeTarget{}, fmt.Errorf("kernel attach smoke only permits userspace probes, got %q", probe.Kind)
			}
			return bpfts.UprobeTarget{
				Path:   targetPath,
				Symbol: probe.Target,
			}, nil
		},
	})
	if err != nil {
		fatal(err)
	}
	if err := runtime.Close(); err != nil {
		fatal(fmt.Errorf("close attached bpf-ts runtime: %w", err))
	}
	fmt.Printf("validated kernel load and %d probe attaches from %s against %s\n", len(manifest.Probes), objectPath, targetPath)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
