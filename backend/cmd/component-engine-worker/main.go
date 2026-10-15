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
	shared := flag.Bool("shared", false, "use inherited memfd and eventfd event data plane")
	privilegedSupervisor := flag.Bool("privileged-supervisor", false, "verify a root Collector with a non-root Engine")
	flag.Parse()
	if *socket == "" || flag.NArg() != 0 || (*privilegedSupervisor && !*shared) {
		fmt.Fprintln(os.Stderr, "usage: component-engine-worker --socket <private-path>")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	if *shared && *privilegedSupervisor {
		err = componentproc.RunWorkerSharedPrivilegedSupervisor(ctx, *socket)
	} else if *shared {
		err = componentproc.RunWorkerShared(ctx, *socket)
	} else {
		err = componentproc.RunWorker(ctx, *socket)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "component Engine worker:", err)
		os.Exit(1)
	}
}
