//go:build linux

// jsplugin-worker executes explicitly requested metadata hooks in a separate
// non-root, sandboxed service. It has no backend or eBPF dependencies.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"agent-ebpf-filter/app/jsplugins"
)

func main() {
	socket := flag.String("socket", "", "absolute private Unix socket path")
	flag.Parse()
	if flag.NArg() != 0 || !filepath.IsAbs(*socket) || *socket == "/" {
		fmt.Fprintln(os.Stderr, "usage: jsplugin-worker --socket /absolute/private/worker.sock")
		os.Exit(2)
	}
	if os.Geteuid() == 0 {
		fmt.Fprintln(os.Stderr, "refusing to run JavaScript worker as root")
		os.Exit(1)
	}
	if _, err := os.Lstat(*socket); !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "socket path already exists or is inaccessible")
		os.Exit(1)
	}
	listener, err := net.Listen("unix", *socket)
	if err != nil { fmt.Fprintln(os.Stderr, "listen:", err); os.Exit(1) }
	defer listener.Close()
	if err := os.Chmod(*socket, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "protect socket:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := jsplugins.NewWorker().Serve(ctx, listener); err != nil {
		fmt.Fprintln(os.Stderr, "JavaScript worker:", err)
		os.Exit(1)
	}
}
