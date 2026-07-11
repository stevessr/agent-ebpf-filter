package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type testListener struct {
	connections chan net.Conn
	closed      chan struct{}
	closeOnce   sync.Once
}

func newTestListener() *testListener {
	return &testListener{
		connections: make(chan net.Conn, 1),
		closed:      make(chan struct{}),
	}
}

func (l *testListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connections:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *testListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (l *testListener) Addr() net.Addr { return testAddr("test-listener") }

type testAddr string

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return string(a) }

func TestListenBackendReturnsConfiguredListener(t *testing.T) {
	const port = 18443
	t.Setenv("AGENT_BACKEND_PORT", strconv.Itoa(port))
	wantListener := newTestListener()
	originalListen := listenTCP
	listenTCP = func(network, address string) (net.Listener, error) {
		if network != "tcp" || address != ":18443" {
			t.Fatalf("listen called with %q %q", network, address)
		}
		return wantListener, nil
	}
	t.Cleanup(func() { listenTCP = originalListen })

	listener, actualPort, err := listenBackend()
	if err != nil {
		t.Fatalf("listenBackend: %v", err)
	}
	defer listener.Close()
	if listener != wantListener {
		t.Fatal("listenBackend returned an unexpected listener")
	}
	if actualPort != port {
		t.Fatalf("actual port = %d, want %d", actualPort, port)
	}
}

func TestListenBackendReturnsConfiguredBindError(t *testing.T) {
	const port = 18444
	t.Setenv("AGENT_BACKEND_PORT", strconv.Itoa(port))
	wantErr := errors.New("address already in use")
	originalListen := listenTCP
	listenTCP = func(_, _ string) (net.Listener, error) { return nil, wantErr }
	t.Cleanup(func() { listenTCP = originalListen })

	listener, actualPort, err := listenBackend()
	if listener != nil {
		listener.Close()
		t.Fatal("listenBackend unexpectedly returned a listener")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("listenBackend error = %v, want %v", err, wantErr)
	}
	if actualPort != 0 {
		t.Fatalf("actual port = %d, want 0", actualPort)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(port)) {
		t.Fatalf("bind error %q does not mention port %d", err, port)
	}
}

func TestServeHTTPServerDrainsRequestOnCancellation(t *testing.T) {
	listener := newTestListener()
	serverConn, clientConn := net.Pipe()
	listener.connections <- serverConn
	defer clientConn.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "ok")
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveHTTPServer(ctx, server, listener, time.Second)
	}()

	requestDone := make(chan error, 1)
	go func() {
		if _, err := fmt.Fprint(clientConn, "GET / HTTP/1.1\r\nHost: test\r\nConnection: close\r\n\r\n"); err != nil {
			requestDone <- err
			return
		}
		res, err := http.ReadResponse(bufio.NewReader(clientConn), &http.Request{Method: http.MethodGet})
		if err == nil {
			defer res.Body.Close()
			_, err = io.ReadAll(res.Body)
		}
		requestDone <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not reach handler")
	}
	cancel()
	select {
	case err := <-done:
		t.Fatalf("server stopped before active request drained: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveHTTPServer: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop after request drained")
	}
	if err := <-requestDone; err != nil {
		t.Fatalf("request failed during graceful shutdown: %v", err)
	}
}

func TestServeHTTPServerForcesCloseAfterShutdownTimeout(t *testing.T) {
	listener := newTestListener()
	serverConn, clientConn := net.Pipe()
	listener.connections <- serverConn
	defer clientConn.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveHTTPServer(ctx, server, listener, 20*time.Millisecond)
	}()
	go func() {
		_, _ = fmt.Fprint(clientConn, "GET / HTTP/1.1\r\nHost: test\r\n\r\n")
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not reach handler")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "shutdown HTTP server") {
			t.Fatalf("shutdown error = %v", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("shutdown error = %v, want context deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not force-close after shutdown timeout")
	}
	close(release)
}
