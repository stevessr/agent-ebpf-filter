// Package componentipc defines an opt-in, versioned local IPC boundary for
// future separately deployed Agent eBPF components. It does not alter the
// current in-process runtime or existing desktop/wrapper socket protocols.
//
// A connection is authenticated in both directions, with an OS peer identity
// verifier and a fresh HMAC challenge. Messages are bounded and role-scoped.
package componentipc

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"agent-ebpf-filter/udsframe"
)

const (
	Version                 uint16 = 1
	MaxMessageBytes                = 1 << 20
	MaxHandshakeBytes              = 128
	DefaultHandshakeTimeout        = 5 * time.Second
	DefaultWriteTimeout            = time.Second
)

var (
	ErrInvalidConfig   = errors.New("invalid component IPC configuration")
	ErrAuthentication  = errors.New("component IPC authentication failed")
	ErrVersionMismatch = errors.New("component IPC version mismatch")
	ErrRoleDenied      = errors.New("component IPC role not authorized")
	ErrFrameDenied     = errors.New("component IPC message type not authorized")
	ErrFrameTooLarge   = errors.New("component IPC message exceeds limit")
)

type Role byte

const (
	RoleCollector  Role = 1
	RoleEngine     Role = 2
	RoleController Role = 3
	RoleGateway    Role = 4
)

func (r Role) Valid() bool {
	return r >= RoleCollector && r <= RoleGateway
}

type Kind byte

const (
	KindEvent     Kind = 1
	KindAlert     Kind = 2
	KindStatus    Kind = 3
	KindAck       Kind = 4
	KindHeartbeat Kind = 5
)

const (
	kindHello byte = 240
	kindAuth  byte = 241
	kindReady byte = 242
)

// CanSend enforces a first-stage least-privilege component wire contract.
// No policy mutation or privileged command message type is defined yet.
func CanSend(role Role, kind Kind) bool {
	switch kind {
	case KindAck, KindHeartbeat:
		return role.Valid()
	case KindStatus:
		return role.Valid()
	case KindEvent:
		return role == RoleCollector
	case KindAlert:
		return role == RoleEngine
	default:
		return false
	}
}

type Message struct {
	Kind Kind
	// Payload aliases internal frame storage from Session.Receive; it must
	// be processed or copied before the next Receive call.
	Payload []byte
}

// Session represents a successfully authenticated full-duplex connection.
// Calls to Send are serialized; only one goroutine should call Receive.
type Session struct {
	conn    net.Conn
	local   Role
	peer    Role
	writeMu sync.Mutex
	readBuf []byte
}

func (s *Session) LocalRole() Role {
	if s == nil {
		return 0
	}
	return s.local
}
func (s *Session) PeerRole() Role {
	if s == nil {
		return 0
	}
	return s.peer
}
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	return s.conn.Close()
}

// Send serializes a nonempty payload, bounded before touching the socket.
// Blocking writes have a deadline, so one slow reader cannot halt a writer.
func (s *Session) Send(kind Kind, payload []byte) error {
	if s == nil || s.conn == nil {
		return ErrInvalidConfig
	}
	if !CanSend(s.local, kind) {
		return fmt.Errorf("%w: role=%d kind=%d", ErrFrameDenied, s.local, kind)
	}
	if len(payload) == 0 || len(payload) > MaxMessageBytes {
		return fmt.Errorf("%w: %d", ErrFrameTooLarge, len(payload))
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.SetWriteDeadline(time.Now().Add(DefaultWriteTimeout)); err != nil {
		_ = s.conn.Close()
		return err
	}
	err := udsframe.WriteTyped(s.conn, byte(kind), payload)
	if err != nil {
		// A partial write can desynchronize the frame stream: fail closed.
		_ = s.conn.Close()
		return err
	}
	return s.conn.SetWriteDeadline(time.Time{})
}

// Receive enforces a strict per-message maximum and the authenticated peer's
// role permissions. The returned payload is borrowed and valid until next call.
func (s *Session) Receive() (Message, error) {
	if s == nil || s.conn == nil {
		return Message{}, ErrInvalidConfig
	}
	// One byte of typed framing precedes the bounded message body.
	raw, err := udsframe.ReadLimitInto(s.conn, s.readBuf, MaxMessageBytes+1)
	if err != nil {
		// Reject truncated or oversized frames without attempting resync.
		_ = s.conn.Close()
		return Message{}, err
	}
	s.readBuf = raw
	if len(raw) < 2 {
		_ = s.conn.Close()
		return Message{}, ErrFrameTooLarge
	}
	kind := Kind(raw[0])
	if !CanSend(s.peer, kind) {
		_ = s.conn.Close()
		return Message{}, fmt.Errorf("%w: peer=%d kind=%d", ErrFrameDenied, s.peer, kind)
	}
	return Message{Kind: kind, Payload: raw[1:]}, nil
}
