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

const (
	defaultBackendURL   = "http://127.0.0.1:8080"
	internalBackendFlag = "--internal-backend"
	askpassModeEnv      = "AGENT_RENEW_ASKPASS"
)

var mainWindow *mygo.Window
var windowMu sync.Mutex

func main() {
	if handled, err := runInternalBackendIfRequested(os.Args[1:]); handled {
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	if isAskpassInvocation() {
		if err := runAskpass(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	runDesktop()
}

func runDesktop() {
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

	app := newRenewApp(backend)
	mygo.App.OnSecondInstance(func(_ []string, _ string) {
		windowMu.Lock()
		window := mainWindow
		windowMu.Unlock()
		if window != nil {
			window.Restore()
			window.Focus()
		}
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
		app.win = window
		windowMu.Lock()
		mainWindow = window
		windowMu.Unlock()

		go app.bootstrap(ctx)
	})

	if err := mygo.App.Run(); err != nil {
		log.Printf("[renew] %v", err)
	}
}

func internalBackendArgs(args []string) (bool, []string) {
	for i, arg := range args {
		if arg != internalBackendFlag {
			continue
		}
		out := make([]string, 0, len(args)-1)
		out = append(out, args[:i]...)
		out = append(out, args[i+1:]...)
		return true, out
	}
	return false, args
}

func (a *renewApp) bootstrap(ctx context.Context) {
	startupCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	session, startupErr := ensureBackend(startupCtx, a.backend)
	cancel()
	if session != nil {
		defer session.Close()
	}
	if ctx.Err() != nil {
		return
	}
	if startupErr != nil {
		a.update(func() {
			a.starting = false
			a.lastErr = startupErr.Error()
		})
		return
	}

	token := strings.TrimSpace(os.Getenv("AGENT_API_TOKEN"))
	if token == "" && session != nil {
		token = session.token
	}
	if token == "" {
		token = existingLocalToken(a.backend)
	}
	a.client = newAPIClient(a.backend, token)
	a.update(func() {
		a.starting = false
		a.connected = true
		a.lastErr = ""
	})
	a.runPolling(ctx, session)
}

func (a *renewApp) update(fn func()) {
	if a.win == nil {
		return
	}
	a.win.Update(fn)
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

// Kept as a compatibility helper for links from the native desktop to the
// full browser workbench.
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
