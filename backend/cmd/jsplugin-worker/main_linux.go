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
	"time"
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
	// Socket access is the authorization boundary for sending JavaScript
	// source. Refuse shared or symlinked directories even when invoked
	// manually rather than through the hardened systemd unit.
	directory := filepath.Dir(*socket)
	dirInfo, err := os.Lstat(directory)
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode().Perm()&0o077 != 0 {
		fmt.Fprintln(os.Stderr, "socket directory must be a private 0700 directory")
		os.Exit(1)
	}
	owner, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Geteuid()) {
		fmt.Fprintln(os.Stderr, "socket directory must be owned by the worker")
		os.Exit(1)
	}
	// Prevent a brief overly-permissive socket between bind and chmod.
	syscall.Umask(0o077)

	// Lock the private directory before examining an orphaned socket.
	// The lock survives until process exit and serializes worker restarts.
	lockPath := filepath.Join(directory, ".worker.lock")
	fd, err := syscall.Open(lockPath, syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_CREAT|syscall.O_RDWR, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open private worker lock:", err)
		os.Exit(1)
	}
	defer syscall.Close(fd)
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		fmt.Fprintln(os.Stderr, "socket path already exists or worker lock is held")
		os.Exit(1)
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)

	// Recover only a *socket* owned by this worker UID, and only when a
	// connection attempt confirms no live listener. Never remove arbitrary
	// files, active sockets or foreign-owned paths.
	if info, err := os.Lstat(*socket); err == nil {
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || info.Mode()&os.ModeSocket == 0 || st.Uid != uint32(os.Geteuid()) {
			fmt.Fprintln(os.Stderr, "socket path already exists or is inaccessible")
			os.Exit(1)
		}
		conn, dialErr := net.DialTimeout("unix", *socket, 60*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			fmt.Fprintln(os.Stderr, "socket path already exists or is inaccessible")
			os.Exit(1)
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			fmt.Fprintln(os.Stderr, "unable to verify orphaned socket:", dialErr)
			os.Exit(1)
		}
		if err := os.Remove(*socket); err != nil {
			fmt.Fprintln(os.Stderr, "remove verified orphaned socket:", err)
			os.Exit(1)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
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
