package app

import (
	"agent-ebpf-filter/app/platform"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var listenTCP = net.Listen

func listenBackend() (net.Listener, int, error) {
	if socket := strings.TrimSpace(os.Getenv("AGENT_DESKTOP_API_SOCKET")); socket != "" && os.Getenv("AGENT_DESKTOP_LIFETIME_SOCKET") != "" {
		if !strings.HasSuffix(socket, ".sock") || !strings.HasPrefix(socket, filepath.Dir(os.Getenv("AGENT_DESKTOP_LIFETIME_SOCKET"))+string(os.PathSeparator)) {
			return nil, 0, fmt.Errorf("desktop API socket must be a sibling of the lifetime socket")
		}
		listener, err := net.Listen("unix", socket)
		if err != nil {
			return nil, 0, fmt.Errorf("listen desktop Unix API: %w", err)
		}
		// Socket lives in a private 0700 user-owned session directory; the
		// privileged child must allow that desktop owner to connect.
		if err := os.Chmod(socket, 0666); err != nil {
			_ = listener.Close()
			_ = os.Remove(socket)
			return nil, 0, fmt.Errorf("permit desktop Unix API: %w", err)
		}
		return listener, 0, nil // no TCP listener, port file, or cluster heartbeat
	}
	startPort, maxTries := 8080, 10
	if rawPort := strings.TrimSpace(os.Getenv("AGENT_BACKEND_PORT")); rawPort != "" {
		if configuredPort, err := strconv.Atoi(rawPort); err == nil && configuredPort > 0 {
			startPort = configuredPort
			maxTries = 1
		} else {
			log.Printf("[WARN] ignoring invalid AGENT_BACKEND_PORT=%q", rawPort)
		}
	}
	var lastErr error
	for i := 0; i < maxTries; i++ {
		port := startPort + i
		l, err := listenTCP("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			return l, port, nil
		}
		lastErr = err
	}
	return nil, 0, fmt.Errorf("listen on backend ports %d-%d: %w", startPort, startPort+maxTries-1, lastErr)
}

func serveHTTPServer(
	ctx context.Context,
	server *http.Server,
	listener net.Listener,
	shutdownTimeout time.Duration,
) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		err := <-serveErr
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func configureRuntimePort(ctx context.Context, jobs *runtimeBackgroundJobs, port int) {
	clusterManagerStore.ConfigurePort(port)
	platform.WritePortFile(port)
	if jobs != nil {
		jobs.Go(func() { runConfiguredClusterHeartbeatLoop(ctx) })
	}
}
