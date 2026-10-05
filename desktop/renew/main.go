package main

import (
	"context"
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/egoist/mygo"
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
	uiOrigin := backend
	if devUI := strings.TrimSpace(os.Getenv("AGENT_RENEW_UI_URL")); devUI != "" {
		if _, err := localBackendPort(devUI); err != nil {
			log.Fatal("Renew development UI must be a local HTTP origin")
		}
		uiOrigin = devUI
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		log.Fatal(err)
	}
	renewURL := joinRenewURL(uiOrigin)
	mygo.App.OnQuit(stop)

	mygo.App.WhenReady(func() {
		loading := mygo.NewWindow(mygo.WindowOptions{Title: "Renew · 启动后端", Width: 1180, Height: 760})
		loading.Page().LoadHTML(strings.Replace(unavailablePage(backend), "Agent eBPF Filter 后端未就绪", "正在启动 Agent eBPF Filter 后端…", 1), "")
		windowMu.Lock()
		mainWindow = loading
		windowMu.Unlock()
		go func() {
			startupCtx, cancelStartup := context.WithTimeout(ctx, 5*time.Minute)
			session, startupErr := ensureBackend(startupCtx, backend, resources)
			cancelStartup()
			defer session.Close()
			if ctx.Err() != nil {
				return
			}
			preload := tokenPreload(uiOrigin, existingLocalToken(backend))
			if session != nil {
				preload = tokenPreload(uiOrigin, session.token)
			}
			window := openMainWindow(renewURL, backend, preload, startupErr)
			windowMu.Lock()
			mainWindow = window
			windowMu.Unlock()
			loading.Close()
			<-ctx.Done()
		}()
	})

	if err := mygo.App.Run(); err != nil {
		log.Printf("[renew] %v", err) // Return normally so the backend session closes.
	}
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

func openMainWindow(renewURL, backend, preload string, startupErr error) *mygo.Window {
	options := mygo.WindowOptions{
		Title:           "Renew",
		URL:             renewURL,
		Width:           1180,
		Height:          760,
		MinWidth:        900,
		MinHeight:       620,
		StateKey:        "main",
		AutoHideMenuBar: true,
		BackgroundColor: "#f6f7f9",
		Page:            mygo.PageOptions{PreloadScript: preload},
	}

	if startupErr == nil && backendAvailable(renewURL) {
		return mygo.NewWindow(options)
	}

	options.URL = ""
	window := mygo.NewWindow(options)
	page := unavailablePage(backend)
	if startupErr != nil {
		log.Printf("[renew] %v", startupErr)
		page = strings.Replace(page, "</main>", "<p>"+html.EscapeString(startupErr.Error())+"</p></main>", 1)
	}
	window.Page().LoadHTML(page, "")
	return window
}

func backendAvailable(target string) bool {
	client := &http.Client{Timeout: 900 * time.Millisecond}
	response, err := client.Get(target)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 500
}

func unavailablePage(backend string) string {
	safeBackend := html.EscapeString(backend)
	return `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8" />
<meta name="color-scheme" content="light dark" />
<title>Renew · Backend unavailable</title>
<style>
:root { font-family: system-ui, sans-serif; color: #26282d; background: #f6f7f9; }
body { min-height: 100vh; margin: 0; display: grid; place-items: center; }
main { width: min(560px, calc(100vw - 48px)); background: white; border: 1px solid #e7e9ee; border-radius: 16px; padding: 28px; box-shadow: 0 8px 32px #0000000a; }
mark { display: inline-grid; place-items: center; width: 40px; height: 40px; border-radius: 12px; background: #e97932; color: white; font-weight: 800; }
h1 { margin: 20px 0 8px; font-size: 22px; }
p { margin: 7px 0; color: #737986; line-height: 1.55; }
code { padding: 3px 6px; border-radius: 6px; background: #f2f3f5; color: #42464f; }
</style>
</head>
<body>
<main>
  <mark>R</mark>
  <h1>Agent eBPF Filter 后端未就绪</h1>
  <p>Renew 已尝试启动随包后端。请完成系统授权；若已取消授权，可重新打开 Renew 再试。也可以连接已运行的系统服务。</p>
  <p>当前后端：<code>` + safeBackend + `</code></p>
  <p>也可以通过 <code>AGENT_BACKEND_URL</code> 或 <code>--backend</code> 指向其他本地实例。</p>
</main>
</body>
</html>`
}
