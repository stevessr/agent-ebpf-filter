package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const defaultBackendURL = "http://127.0.0.1:8080"

var mainWindow *mygo.Window
var windowMu sync.Mutex

func main() {
	backendFlag := flag.String("backend", "", "Agent eBPF Filter backend URL")
	flag.Parse()

	backend, err := resolveBackendURL(*backendFlag)
	if err != nil {
		log.Fatal(err)
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	mygo.App.OnQuit(stop)

	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		log.Fatal(err)
	}

	app := newNativeApp(backend)
	mygo.App.OnSecondInstance(func(_ []string, _ string) {
		windowMu.Lock()
		window := mainWindow
		windowMu.Unlock()
		if window == nil {
			return
		}
		window.Restore()
		window.Focus()
	})

	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{
			Title:     "Renew",
			Width:     1180,
			Height:    760,
			MinWidth:  900,
			MinHeight: 620,
			StateKey:  "main",
			Content:   ui.View(app.view),
		})
		windowMu.Lock()
		mainWindow = window
		windowMu.Unlock()
		app.setWindow(window)

		go runNativeClient(ctx, app, backend, resources)
	})

	if err := mygo.App.Run(); err != nil {
		log.Printf("[renew] %v", err)
	}
}

func runNativeClient(ctx context.Context, app *nativeApp, backend, resources string) {
	app.setStartup("正在连接 Agent eBPF Filter 后端…", "")
	startupCtx, cancelStartup := context.WithTimeout(ctx, 5*time.Minute)
	session, err := ensureBackend(startupCtx, backend, resources)
	cancelStartup()
	if session != nil {
		defer session.Close()
	}
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		app.setStartup("后端未就绪", err.Error())
		return
	}

	token := resolveClientTokenFromEnvironment()
	if session != nil && session.token != "" {
		token = session.token
	} else if token == "" {
		token = existingLocalToken(backend)
	}

	client, err := newAPIClient(backend, token)
	if err != nil {
		app.setStartup("无法创建后端客户端", err.Error())
		return
	}
	app.setClient(client)
	app.setStartup("后端已连接", "")
	app.run(ctx, client)
}


func resolveClientTokenFromEnvironment() string {
	for _, key := range []string{"AGENT_API_KEY", "AGENT_ACCESS_TOKEN", "AGENT_EBPF_ACCESS_TOKEN"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func resolveBackendURL(explicit string) (string, error) {
	raw := strings.TrimSpace(explicit)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("AGENT_BACKEND_URL"))
	}
	if raw == "" {
		raw = defaultBackendURL
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("invalid backend URL %q", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("backend URL must use http or https")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func joinRenewURL(backend string) string {
	parsed, err := url.Parse(backend)
	if err != nil {
		return backend + "/renew"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/renew"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
