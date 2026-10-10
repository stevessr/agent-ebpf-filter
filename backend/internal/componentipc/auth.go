package componentipc

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"agent-ebpf-filter/udsframe"
)

// Verifier must check the authenticated operating-system peer identity.
// The production Unix implementation uses Linux SO_PEERCRED and explicit UIDs.
// A nil verifier is rejected, even if a shared secret was configured.
type Verifier func(net.Conn) error

type ServerConfig struct {
	Secret       []byte
	Role         Role
	AllowedPeers []Role
	VerifyPeer   Verifier
}

type ClientConfig struct {
	Secret         []byte
	Role           Role
	ExpectedServer Role
	VerifyPeer     Verifier
}

// HandshakeServer rejects unexpected OS peers, protocol versions, and roles.
// Both challenge/response MACs bind the nonce and both side roles to the
// protocol version. A failed handshake always closes the supplied connection.
func HandshakeServer(conn net.Conn, cfg ServerConfig) (_ *Session, err error) {
	if conn == nil {
		return nil, ErrInvalidConfig
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	if len(cfg.Secret) < 32 || !cfg.Role.Valid() || cfg.VerifyPeer == nil || len(cfg.AllowedPeers) == 0 {
		return nil, ErrInvalidConfig
	}
	if err = cfg.VerifyPeer(conn); err != nil {
		return nil, fmt.Errorf("%w: peer identity: %v", ErrAuthentication, err)
	}
	if err = conn.SetDeadline(time.Now().Add(DefaultHandshakeTimeout)); err != nil {
		return nil, err
	}
	defer conn.SetDeadline(time.Time{})
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	var hello [35]byte
	binary.BigEndian.PutUint16(hello[0:2], Version)
	hello[2] = byte(cfg.Role)
	copy(hello[3:], nonce[:])
	if err = udsframe.WriteTyped(conn, kindHello, hello[:]); err != nil {
		return nil, err
	}
	frame, readErr := udsframe.ReadLimit(conn, MaxHandshakeBytes)
	if readErr != nil {
		return nil, readErr
	}
	if len(frame) != 36 || frame[0] != kindAuth {
		return nil, ErrAuthentication
	}
	if binary.BigEndian.Uint16(frame[1:3]) != Version {
		return nil, ErrVersionMismatch
	}
	clientRole := Role(frame[3])
	allowed := false
	for _, role := range cfg.AllowedPeers {
		if role == clientRole && role.Valid() {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, ErrRoleDenied
	}
	expected := challengeMAC(cfg.Secret, "client", nonce[:], clientRole, cfg.Role)
	if !hmac.Equal(frame[4:36], expected[:]) {
		return nil, ErrAuthentication
	}
	ready := challengeMAC(cfg.Secret, "server", nonce[:], clientRole, cfg.Role)
	if err = udsframe.WriteTyped(conn, kindReady, ready[:]); err != nil {
		return nil, err
	}
	return &Session{conn: conn, local: cfg.Role, peer: clientRole}, nil
}

// HandshakeClient verifies the local socket owner before trusting the server
// nonce and finally validates that the server possesses the shared secret.
// No credential is transmitted in clear text on the socket.
func HandshakeClient(conn net.Conn, cfg ClientConfig) (_ *Session, err error) {
	if conn == nil {
		return nil, ErrInvalidConfig
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	if len(cfg.Secret) < 32 || !cfg.Role.Valid() || !cfg.ExpectedServer.Valid() || cfg.VerifyPeer == nil {
		return nil, ErrInvalidConfig
	}
	if err = cfg.VerifyPeer(conn); err != nil {
		return nil, fmt.Errorf("%w: peer identity: %v", ErrAuthentication, err)
	}
	if err = conn.SetDeadline(time.Now().Add(DefaultHandshakeTimeout)); err != nil {
		return nil, err
	}
	defer conn.SetDeadline(time.Time{})
	frame, readErr := udsframe.ReadLimit(conn, MaxHandshakeBytes)
	if readErr != nil {
		return nil, readErr
	}
	if len(frame) != 36 || frame[0] != kindHello {
		return nil, ErrAuthentication
	}
	if binary.BigEndian.Uint16(frame[1:3]) != Version {
		return nil, ErrVersionMismatch
	}
	if Role(frame[3]) != cfg.ExpectedServer {
		return nil, ErrRoleDenied
	}
	nonce := frame[4:36]
	var auth [35]byte
	binary.BigEndian.PutUint16(auth[:2], Version)
	auth[2] = byte(cfg.Role)
	mac := challengeMAC(cfg.Secret, "client", nonce, cfg.Role, cfg.ExpectedServer)
	copy(auth[3:], mac[:])
	if err = udsframe.WriteTyped(conn, kindAuth, auth[:]); err != nil {
		return nil, err
	}
	ready, readErr := udsframe.ReadLimit(conn, MaxHandshakeBytes)
	if readErr != nil {
		return nil, readErr
	}
	if len(ready) != 33 || ready[0] != kindReady {
		return nil, ErrAuthentication
	}
	expected := challengeMAC(cfg.Secret, "server", nonce, cfg.Role, cfg.ExpectedServer)
	if !hmac.Equal(ready[1:], expected[:]) {
		return nil, ErrAuthentication
	}
	return &Session{conn: conn, local: cfg.Role, peer: cfg.ExpectedServer}, nil
}

func challengeMAC(secret []byte, phase string, nonce []byte, client, server Role) [32]byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("agent-ebpf/componentipc/v1/"))
	_, _ = mac.Write([]byte(phase))
	_, _ = mac.Write(nonce)
	_, _ = mac.Write([]byte{byte(Version >> 8), byte(Version), byte(client), byte(server)})
	var digest [32]byte
	copy(digest[:], mac.Sum(nil))
	return digest
}
