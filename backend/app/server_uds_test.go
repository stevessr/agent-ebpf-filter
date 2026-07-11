package app

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestServeUDSListenerClosesConnectionsOnCancellation(t *testing.T) {
	listener := newTestListener()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	listener.connections <- serverConn

	ctx, cancel := context.WithCancel(context.Background())
	verified := make(chan struct{})
	done := make(chan struct{})
	go func() {
		serveUDSListener(ctx, listener, make(chan *pb.Event, 1), func(net.Conn) error {
			close(verified)
			return nil
		})
		close(done)
	}()

	select {
	case <-verified:
	case <-time.After(time.Second):
		t.Fatal("UDS connection was not accepted")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("UDS listener did not stop after cancellation")
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := clientConn.Read(make([]byte, 1)); err == nil {
		t.Fatal("UDS peer remained open after server cancellation")
	}
}

func TestUDSPathMatchesWrapperContract(t *testing.T) {
	if udsPath != "/tmp/agent-ebpf.sock" {
		t.Fatalf("udsPath = %q, want wrapper contract path", udsPath)
	}
}

func TestRemoveUDSSocketIfSamePreservesReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.sock")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	original, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat original: %v", err)
	}
	replacementPath := path + ".replacement"
	if err := os.WriteFile(replacementPath, []byte("replacement"), 0600); err != nil {
		t.Fatalf("write replacement: %v", err)
	}
	if err := os.Rename(replacementPath, path); err != nil {
		t.Fatalf("replace original: %v", err)
	}

	if err := removeUDSSocketIfSame(path, original); err != nil {
		t.Fatalf("removeUDSSocketIfSame: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "replacement" {
		t.Fatalf("replacement = %q, err=%v", data, err)
	}
}

func TestRemoveUDSSocketIfSameRemovesOwnedPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.sock")
	if err := os.WriteFile(path, []byte("owned"), 0600); err != nil {
		t.Fatalf("write path: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat path: %v", err)
	}
	if err := removeUDSSocketIfSame(path, info); err != nil {
		t.Fatalf("removeUDSSocketIfSame: %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path still exists: %v", err)
	}
}
