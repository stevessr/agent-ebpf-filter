//go:build linux

package componentproc

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"agent-ebpf-filter/internal/componentipc"
)

var (
	ErrUnsafeSupervisor  = errors.New("root supervisor must not launch a same-UID Engine")
	ErrInvalidSupervisor = errors.New("invalid component supervisor configuration")
)

// RunEngine starts exactly one non-root child via an explicit absolute
// executable path and an isolated Unix-domain socket. The parent owns the
// socket, passes a fresh random credential over inherited FD3, authenticates
// the child's explicit UID and joins or terminates the process before return.
//
// The callback runs inside the parent and may send pre-normalized/redacted
// events through Session.SendSequenced. This is a rootless integration boundary,
// NOT the privileged production Collector path or a promise of privilege drop.
func RunEngine(ctx context.Context, executable string, publish func(*componentipc.Session) error) error {
	if ctx == nil || executable == "" || !filepath.IsAbs(executable) || publish == nil {
		return ErrInvalidSupervisor
	}
	if os.Geteuid() == 0 {
		return ErrUnsafeSupervisor
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(executable)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return ErrInvalidSupervisor
	}
	dir, err := os.MkdirTemp("", "agent-ebpf-components-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	socketPath := filepath.Join(dir, "engine.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(socketPath, 0600); err != nil {
		return err
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer reader.Close()
	defer writer.Close()
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return err
	}
	defer clear(secret[:])

	childCtx, stopChild := context.WithCancel(ctx)
	defer stopChild()
	command := exec.CommandContext(childCtx, executable, "--socket", socketPath)
	// A minimal environment avoids leaking supervisor-only runtime secrets.
	command.Env = []string{"LANG=C"}
	command.ExtraFiles = []*os.File{reader}
	command.WaitDelay = 2 * time.Second
	if err := command.Start(); err != nil {
		return fmt.Errorf("launch Engine: %w", err)
	}
	_ = reader.Close()
	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	waitChild := func() error {
		stopChild()
		_ = listener.Close()
		return <-waitDone
	}
	if _, err := writer.Write(secret[:]); err != nil {
		_ = writer.Close()
		_ = waitChild()
		return fmt.Errorf("pass inherited Engine credential: %w", err)
	}
	_ = writer.Close()
	// Ensure a crashed child or cancelled supervisor cannot strand Accept.
	accepted := make(chan struct {
		conn *net.UnixConn
		err  error
	}, 1)
	go func() {
		conn, acceptErr := listener.AcceptUnix()
		accepted <- struct {
			conn *net.UnixConn
			err  error
		}{conn: conn, err: acceptErr}
	}()
	stopAccept := context.AfterFunc(childCtx, func() { _ = listener.Close() })
	defer stopAccept()

	var conn *net.UnixConn
	select {
	case result := <-accepted:
		if result.err != nil {
			_ = waitChild()
			return fmt.Errorf("accept Engine: %w", result.err)
		}
		conn = result.conn
	case childErr := <-waitDone:
		stopChild()
		_ = listener.Close()
		return fmt.Errorf("Engine exited before IPC authentication: %w", childErr)
	case <-ctx.Done():
		_ = waitChild()
		return ctx.Err()
	}
	defer conn.Close()
	stopSession := context.AfterFunc(childCtx, func() { _ = conn.Close() })
	defer stopSession()

	session, err := componentipc.HandshakeServer(conn, componentipc.ServerConfig{
		Secret: secret[:], Role: componentipc.RoleCollector,
		AllowedPeers: []componentipc.Role{componentipc.RoleEngine},
		VerifyPeer:   componentipc.VerifyUnixPeerUIDs(uint32(os.Geteuid())),
	})
	if err != nil {
		_ = waitChild()
		return fmt.Errorf("authenticate Engine: %w", err)
	}
	publishErr := publish(session)
	_ = session.Close()
	if publishErr != nil {
		_ = waitChild()
		return publishErr
	}
	// Normal EOF lets the worker exit on its own. A missing exit cannot
	// block forever; the context deadline remains the hard process bound.
	select {
	case childErr := <-waitDone:
		return childErr
	case <-ctx.Done():
		_ = waitChild()
		return ctx.Err()
	}
}

// SendEvent waits for an explicit processing ACK for a normalized envelope.
// No retry is performed and a write alone never establishes delivery.
func SendEvent(session *componentipc.Session, stream componentipc.StreamID, seq uint64, payload []byte) error {
	if session == nil {
		return componentipc.ErrInvalidConfig
	}
	if err := session.SendSequenced(componentipc.KindEvent, componentipc.Sequenced{
		Stream: stream, Sequence: seq, Payload: payload,
	}); err != nil {
		return err
	}
	record, err := session.ReceiveRecordCopy()
	if err != nil {
		return err
	}
	if record.Kind != componentipc.KindAck {
		return fmt.Errorf("%w: expected Engine ACK", componentipc.ErrFrameDenied)
	}
	if record.Receipt.Stream != stream || record.Receipt.Sequence != seq {
		return fmt.Errorf("%w: unexpected ACK stream or sequence", componentipc.ErrInvalidSequence)
	}
	return nil
}
