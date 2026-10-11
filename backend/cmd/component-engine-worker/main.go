//go:build linux

// component-engine-worker is an unprivileged, explicitly opt-in observer.
// It is not launched by production Agent eBPF or Renew startup yet.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"agent-ebpf-filter/internal/componentproc"
)

func main() {
	socket := flag.String("socket", "", "private supervisor-owned Unix socket")
	flag.Parse()
	if *socket == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: component-engine-worker --socket <private-path>")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := componentproc.RunWorker(ctx, *socket); err != nil {
		fmt.Fprintln(os.Stderr, "component Engine worker:", err)
		os.Exit(1)
	}
}
