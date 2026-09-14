package main

import (
	"fmt"
	"os"

	bpf "agent-ebpf-filter/ebpf"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
)

func main() {
	if err := rlimit.RemoveMemlock(); err != nil {
		fmt.Fprintf(os.Stderr, "remove memlock: %v\n", err)
		os.Exit(1)
	}
	spec, err := bpf.LoadAgentTracker()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load tracker spec: %v\n", err)
		os.Exit(1)
	}
	collection, err := ebpf.NewCollection(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kernel verifier rejected tracker: %v\n", err)
		os.Exit(1)
	}
	collection.Close()
}
