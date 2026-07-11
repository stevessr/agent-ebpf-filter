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
	"strconv"
	"strings"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section startup_server.go ----

var listenTCP = net.Listen

func listenBackend() (net.Listener, int, error) {
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
