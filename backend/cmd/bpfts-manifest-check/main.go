package main

import (
	"fmt"
	"os"

	"agent-ebpf-filter/app/bpfts"
)

func main() {
	if len(os.Args) < 3 || (len(os.Args)-1)%2 != 0 {
		fmt.Fprintln(os.Stderr, "usage: bpfts-manifest-check <manifest.json> <object.o> [<manifest.json> <object.o> ...]")
		os.Exit(2)
	}
	for index := 1; index < len(os.Args); index += 2 {
		manifestPath := os.Args[index]
		objectPath := os.Args[index+1]
		manifest, err := bpfts.LoadManifest(manifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", manifestPath, err)
			os.Exit(1)
		}
		if err := bpfts.ValidateObjectManifest(objectPath, manifest); err != nil {
			fmt.Fprintf(os.Stderr, "%s + %s: %v\n", manifestPath, objectPath, err)
			os.Exit(1)
		}
		fmt.Printf(
			"%s + %s: v%d, %d probes, %d maps, ELF contract OK\n",
			manifestPath,
			objectPath,
			manifest.Version,
			len(manifest.Probes),
			len(manifest.Maps),
		)
	}
}
