package app

import (
	"agent-ebpf-filter/app/platform"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
)

// ---- moved from backend/zz_merged_backend.go section startup_server.go ----

func chooseBackendPort() int {
	startPort, maxTries := 8080, 10
	if rawPort := strings.TrimSpace(os.Getenv("AGENT_BACKEND_PORT")); rawPort != "" {
		if configuredPort, err := strconv.Atoi(rawPort); err == nil && configuredPort > 0 {
			startPort = configuredPort
			maxTries = 1
		} else {
			log.Printf("[WARN] ignoring invalid AGENT_BACKEND_PORT=%q", rawPort)
		}
	}
	actualPort := startPort
	for i := 0; i < maxTries; i++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", startPort+i))
		if err == nil {
			actualPort = startPort + i
			l.Close()
			break
		}
	}
	return actualPort
}

func configureRuntimePort(ctx context.Context, port int) {
	clusterManagerStore.ConfigurePort(port)
	platform.WritePortFile(port)
	startClusterHeartbeatLoop(ctx)
}
