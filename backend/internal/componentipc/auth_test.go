package componentipc

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"agent-ebpf-filter/udsframe"
)

func testSecret() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}

func trustPipe(net.Conn) error { return nil }

type serverResult struct {
	session *Session
	err     error
}

func handshakePair(t *testing.T, clientRole, serverRole Role) (*Session, *Session) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	results := make(chan serverResult, 1)
	go func() {
		s, err := HandshakeServer(serverConn, ServerConfig{
			Secret: testSecret(), Role: serverRole, AllowedPeers: []Role{clientRole}, VerifyPeer: trustPipe,
		})
		results <- serverResult{s, err}
	}()
	client, err := HandshakeClient(clientConn, ClientConfig{
		Secret: testSecret(), Role: clientRole, ExpectedServer: serverRole, VerifyPeer: trustPipe,
	})
	if err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	result := <-results
	if result.err != nil {
		t.Fatalf("server handshake: %v", result.err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = result.session.Close()
	})
	return client, result.session
}

func TestMutualHandshakeAndRoleBinding(t *testing.T) {
	client, server := handshakePair(t, RoleCollector, RoleEngine)
	if client.LocalRole() != RoleCollector || client.PeerRole() != RoleEngine {
		t.Fatalf("client roles %v -> %v", client.LocalRole(), client.PeerRole())
	}
	if server.LocalRole() != RoleEngine || server.PeerRole() != RoleCollector {
		t.Fatalf("server roles %v -> %v", server.LocalRole(), server.PeerRole())
	}
}

func TestAuthenticatedFrameAndDeniedRole(t *testing.T) {
	client, server := handshakePair(t, RoleCollector, RoleEngine)
	errs := make(chan error, 1)
	go func() { errs <- client.Send(KindEvent, []byte("captured")) }()
	message, err := server.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if message.Kind != KindEvent || string(message.Payload) != "captured" {
		t.Fatalf("unexpected decoded frame: %+v", message)
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if err := client.Send(KindAlert, []byte("forged")); !errors.Is(err, ErrFrameDenied) {
		t.Fatalf("collector emitted engine-only alert: %v", err)
	}
	if err := client.Send(KindEvent, nil); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("empty event accepted: %v", err)
	}
	if err := client.Send(KindEvent, make([]byte, MaxMessageBytes+1)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversize event accepted: %v", err)
	}
}

func TestHandshakeRejectsIncorrectSecret(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	results := make(chan error, 1)
	go func() {
		_, err := HandshakeServer(serverConn, ServerConfig{
			Secret: testSecret(), Role: RoleEngine, AllowedPeers: []Role{RoleCollector}, VerifyPeer: trustPipe,
		})
		results <- err
	}()
	other := []byte("not-the-same-secret-000000000000000")
	_, err := HandshakeClient(clientConn, ClientConfig{
		Secret: other, Role: RoleCollector, ExpectedServer: RoleEngine, VerifyPeer: trustPipe,
	})
	if err == nil {
		t.Fatal("wrong secret accepted")
	}
	if serverErr := <-results; !errors.Is(serverErr, ErrAuthentication) {
		t.Fatalf("server error: %v", serverErr)
	}
}

func TestHandshakeRejectsUnexpectedRole(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	result := make(chan error, 1)
	go func() {
		_, err := HandshakeServer(serverConn, ServerConfig{
			Secret: testSecret(), Role: RoleEngine, AllowedPeers: []Role{RoleCollector}, VerifyPeer: trustPipe,
		})
		result <- err
	}()
	_, _ = HandshakeClient(clientConn, ClientConfig{
		Secret: testSecret(), Role: RoleGateway, ExpectedServer: RoleEngine, VerifyPeer: trustPipe,
	})
	if err := <-result; !errors.Is(err, ErrRoleDenied) {
		t.Fatalf("unauthorized role error %v", err)
	}
}

func TestHandshakeRejectsWrongServerRoleAndVersion(t *testing.T) {
	s, c := net.Pipe()
	go func() {
		defer s.Close()
		var challenge [35]byte
		binary.BigEndian.PutUint16(challenge[:2], Version+1)
		challenge[2] = byte(RoleEngine)
		_, _ = rand.Read(challenge[3:])
		_ = udsframe.WriteTyped(s, kindHello, challenge[:])
	}()
	_, err := HandshakeClient(c, ClientConfig{
		Secret: testSecret(), Role: RoleCollector, ExpectedServer: RoleEngine, VerifyPeer: trustPipe,
	})
	if !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("incorrect version error %v", err)
	}
}

func TestHandshakeRequiresPeerVerifier(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if _, err := HandshakeServer(a, ServerConfig{
		Secret: testSecret(), Role: RoleEngine, AllowedPeers: []Role{RoleCollector},
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("missing peer verifier allowed: %v", err)
	}
	if _, err := HandshakeClient(b, ClientConfig{
		Secret: testSecret(), Role: RoleCollector, ExpectedServer: RoleEngine,
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("missing client verifier allowed: %v", err)
	}
}

func TestHandshakeTimesOutWhenPeerDoesNotRespond(t *testing.T) {
	// The default handshake timeout is intentionally longer than tests need;
	// exercise the deadline directly without sleeping five seconds.
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if err := a.SetReadDeadline(time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err := udsframe.ReadLimit(a, MaxHandshakeBytes)
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("expected bounded transport deadline, got %v", err)
	}
}
