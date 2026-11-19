package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func waitForMonitorResult(t *testing.T, app *monitorApp, pred func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app.receiveResults()
		if pred() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	app.receiveResults()
	if !pred() {
		t.Fatal("timed out waiting for monitorApp state transition")
	}
}

func TestConnectHandshakeRejectsInvalidTokenBeforeEnteringHome(t *testing.T) {
	var authed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authed.Load() || r.Header.Get("X-API-KEY") != "good-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/system/collector-health":
			fmt.Fprint(w, `{"captureHealthy":true,"ringbufEventsTotal":15}`)
		case "/events/summaries":
			fmt.Fprint(w, `{"events":[{"eventId":"e2","pid":12,"comm":"curl","decision":"BLOCK","riskScore":0.92}],"nextCursor":"c1","hasMore":true}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	app := newMonitorApp()
	app.origin = server.URL + "/"
	app.token = "bad-token"
	app.connect()

	waitForMonitorResult(t, app, func() bool { return !app.connecting })
	if app.connected || app.client != nil || app.page != "connect" {
		t.Fatalf("failed handshake must stay on connect without an active session: connected=%v client=%v page=%q", app.connected, app.client, app.page)
	}
	if !app.authFailed || !strings.Contains(app.lastErr, "401/403") {
		t.Fatalf("expected auth failure diagnostic, got authFailed=%v lastErr=%q", app.authFailed, app.lastErr)
	}

	authed.Store(true)
	app.token = "good-token"
	app.connect()
	waitForMonitorResult(t, app, func() bool { return !app.connecting && app.connected })
	if app.page != "home" || len(app.snap.Events) != 1 || app.snap.NextCursor != "c1" {
		t.Fatalf("expected successful connect to enter home with snapshot: page=%q snap=%+v", app.page, app.snap)
	}
	if len(app.recentOrigins) != 1 || app.recentOrigins[0] != server.URL {
		t.Fatalf("expected verified origin in recentOrigins, got %+v", app.recentOrigins)
	}
}

func TestPollingBackoffPreservesSnapshotAndLoadsMorePages(t *testing.T) {
	var failHealth atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failHealth.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		switch r.URL.Path {
		case "/system/collector-health":
			fmt.Fprint(w, `{"captureHealthy":true}`)
		case "/events/summaries":
			if r.URL.Query().Get("cursor") == "c1" {
				fmt.Fprint(w, `{"events":[{"eventId":"e1","pid":11,"comm":"bash","decision":"ALLOW","riskScore":0.1}],"nextCursor":""}`)
				return
			}
			fmt.Fprint(w, `{"events":[{"eventId":"e2","pid":12,"comm":"curl","decision":"BLOCK","riskScore":0.9}],"nextCursor":"c1","hasMore":true}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	app := newMonitorApp()
	app.applyOpenURL("agentebpf://connect?origin=" + server.URL + "&token=tok")
	waitForMonitorResult(t, app, func() bool { return app.connected && !app.connecting })

	app.loadMoreEvents()
	waitForMonitorResult(t, app, func() bool { return !app.loadingMore })
	if len(app.snap.Events) != 2 || app.snap.NextCursor != "" {
		t.Fatalf("expected pagination to merge older event: %+v", app.snap.Events)
	}

	failHealth.Store(true)
	app.refreshWithMode(false)
	waitForMonitorResult(t, app, func() bool { return !app.loading })
	if !app.connected || len(app.snap.Events) != 2 || app.consecutiveFailures != 1 {
		t.Fatalf("transient poll failure should preserve connected snapshot: connected=%v events=%d failures=%d",
			app.connected, len(app.snap.Events), app.consecutiveFailures)
	}
	if app.pollInterval() <= basePollInterval {
		t.Fatalf("expected backoff poll interval > %v, got %v", basePollInterval, app.pollInterval())
	}

	app.disconnect()
	if app.connected || app.client != nil || app.token != "" || len(app.snap.Events) != 0 {
		t.Fatalf("disconnect did not clear session/token: connected=%v token=%q", app.connected, app.token)
	}
}

func TestProbeConnectionDoesNotSwitchPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"captureHealthy":true}`)
	}))
	defer server.Close()

	app := newMonitorApp()
	app.origin = strings.TrimPrefix(server.URL, "http://")
	app.probeConnection()
	waitForMonitorResult(t, app, func() bool { return !app.probing })
	if app.page != "connect" || app.connected || !app.probeOK || !strings.Contains(app.probeMsg, "连接验证通过") {
		t.Fatalf("unexpected probe state: page=%q connected=%v probeOK=%v probeMsg=%q lastErr=%q",
			app.page, app.connected, app.probeOK, app.probeMsg, app.lastErr)
	}
}
