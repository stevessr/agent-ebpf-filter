package main

import (
	"fmt"
	"os"

	"agent-ebpf-filter/app/bpfts"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: bpfts-manifest-check <manifest.json> [...]")
		os.Exit(2)
	}
	for _, path := range os.Args[1:] {
		manifest, err := bpfts.LoadManifest(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("%s: v%d, %d probes, %d maps\n", path, manifest.Version, len(manifest.Probes), len(manifest.Maps))
	}
}
