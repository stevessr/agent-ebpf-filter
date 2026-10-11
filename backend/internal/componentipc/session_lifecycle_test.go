package componentipc

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"agent-ebpf-filter/udsframe"
)

var errClearDeadline = errors.New("injected deadline reset failure")

type failWriteDeadlineResetConn struct {
	net.Conn
	closed atomic.Bool
}

func (c *failWriteDeadlineResetConn) SetWriteDeadline(t time.Time) error {
	if t.IsZero() {
		return errClearDeadline
	}
	return c.Conn.SetWriteDeadline(t)
}

func (c *failWriteDeadlineResetConn) Close() error {
	c.closed.Store(true)
	return c.Conn.Close()
}

func TestSendClosesConnectionWhenWriteDeadlineCannotBeReset(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	wrapped := &failWriteDeadlineResetConn{Conn: a}
	session := &Session{conn: wrapped, local: RoleCollector, peer: RoleEngine}
	done := make(chan error, 1)
	go func() {
		frame, err := udsframe.ReadLimit(b, MaxMessageBytes+1)
		if err == nil && (len(frame) != 2 || Kind(frame[0]) != KindEvent || frame[1] != 'x') {
			err = errors.New("unexpected frame")
		}
		done <- err
	}()
	if err := session.Send(KindEvent, []byte("x")); !errors.Is(err, errClearDeadline) {
		t.Fatalf("expected deadline reset failure, got %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !wrapped.closed.Load() {
		t.Fatal("session remained open after failing to reset its write deadline")
	}
}

type failHandshakeDeadlineResetConn struct {
	net.Conn
	closed atomic.Bool
}

func (c *failHandshakeDeadlineResetConn) SetDeadline(t time.Time) error {
	if t.IsZero() {
		return errClearDeadline
	}
	return c.Conn.SetDeadline(t)
}

func (c *failHandshakeDeadlineResetConn) Close() error {
	c.closed.Store(true)
	return c.Conn.Close()
}

func TestHandshakeFailsClosedWhenDeadlineCannotBeReset(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	wrapped := &failHandshakeDeadlineResetConn{Conn: clientConn}
	result := make(chan serverResult, 1)
	go func() {
		session, err := HandshakeServer(serverConn, ServerConfig{
			Secret: testSecret(), Role: RoleEngine,
			AllowedPeers: []Role{RoleCollector}, VerifyPeer: trustPipe,
		})
		result <- serverResult{session: session, err: err}
	}()
	session, err := HandshakeClient(wrapped, ClientConfig{
		Secret: testSecret(), Role: RoleCollector,
		ExpectedServer: RoleEngine, VerifyPeer: trustPipe,
	})
	if session != nil || !errors.Is(err, errClearDeadline) {
		t.Fatalf("uncleared handshake deadline returned session=%v err=%v", session, err)
	}
	if !wrapped.closed.Load() {
		t.Fatal("failed handshake left connection open")
	}
	// A net.Pipe server may still be finishing its last write when the
	// failed client deliberately closes. Either outcome is safe on that end.
	server := <-result
	if server.session != nil {
		defer server.session.Close()
	}
}

func TestEmptySessionClose(t *testing.T) {
	if err := (*Session)(nil).Close(); err != nil {
		t.Fatal(err)
	}
	if err := (&Session{}).Close(); err != nil {
		t.Fatal(err)
	}
}
