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
	"agent-ebpf-filter/internal/componentshm"
	"agent-ebpf-filter/internal/componentwire"
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
	return runEngine(ctx, executable, nil, publish)
}

// RunEngineShared is a rootless opt-in alternative: events use a bounded
// process-shared memfd ring; only cumulative validation ACKs cross authenticated
// UDS. The current embedded backend and Renew never invoke this function.
func RunEngineShared(ctx context.Context, executable string, slots, payload int,
	publish func(*componentipc.Session, *componentshm.Ring) error) error {
	if ctx == nil || publish == nil || !filepath.IsAbs(executable) {
		return ErrInvalidSupervisor
	}
	if os.Geteuid() == 0 {
		return ErrUnsafeSupervisor
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ring, err := componentshm.Create(slots, payload)
	if err != nil {
		return err
	}
	defer ring.Close()
	return runEngine(ctx, executable, ring, func(session *componentipc.Session) error {
		publishErr := publish(session, ring)
		closeErr := ring.Finish()
		if publishErr != nil {
			return publishErr
		}
		return closeErr
	})
}

func runEngine(ctx context.Context, executable string, shared *componentshm.Ring,
	publish func(*componentipc.Session) error) error {
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
	args := []string{"--socket", socketPath}
	extra := []*os.File{reader}
	if shared != nil {
		mem, wake, err := shared.Files()
		if err != nil {
			return err
		}
		args = append(args, "--shared")
		extra = append(extra, mem, wake)
	}
	command := exec.CommandContext(childCtx, executable, args...)
	// A minimal environment avoids leaking supervisor-only runtime secrets.
	command.Env = []string{"LANG=C"}
	command.ExtraFiles = extra
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

// SendEvent strictly encodes a privacy-minimized projection and waits for an
// explicit validation ACK. No retry occurs; a write alone is not delivery.
func SendEvent(session *componentipc.Session, stream componentipc.StreamID, seq uint64, event componentwire.Event) error {
	if session == nil {
		return componentipc.ErrInvalidConfig
	}
	payload, err := componentwire.EncodeEvent(event)
	if err != nil {
		return err
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

// SendSharedBatch validates the entire batch before publishing any record,
// then issues one eventfd wakeup. Authenticated UDS only carries cumulative
// ACKs for schema-validated records; it does not transport event payloads.
// Caller MUST serialize batches: no concurrent producers or UDS ACK readers.
// A timeout/cancellation belongs to the supervisor's context and will close
// the authenticated session. There is no retry, replay or durable delivery.
func SendSharedBatch(session *componentipc.Session, ring *componentshm.Ring,
	events []componentwire.Event) error {
	if session == nil || ring == nil || len(events) == 0 {
		return componentipc.ErrInvalidConfig
	}
	encoded := make([][]byte, 0, len(events))
	for _, event := range events {
		data, err := componentwire.EncodeEvent(event)
		if err != nil {
			return err
		}
		if len(data) > ring.MaxPayload() {
			return componentshm.ErrTooLarge
		}
		encoded = append(encoded, data)
	}
	stats := ring.Stats()
	if stats.Queued > stats.Capacity {
		return componentshm.ErrInvalidRing
	}
	if uint64(len(encoded)) > stats.Capacity-stats.Queued {
		// This rejection is atomic for the whole batch; do not publish any
		// subset of records or silently hide lost admission attempts.
		if err := ring.RejectBatch(uint64(len(encoded))); err != nil {
			return err
		}
		return componentshm.ErrRingFull
	}
	for _, data := range encoded {
		if err := ring.TryWrite(data); err != nil {
			// A failed write may leave only a partially enqueued batch.
			// The caller must abandon this session rather than resubmit.
			return err
		}
	}
	want := ring.Stats().Published
	if err := ring.Notify(); err != nil {
		return err
	}
	stream := componentipc.StreamID(ring.Stream())
	last := stats.Consumed
	for {
		record, err := session.ReceiveRecordCopy()
		if err != nil {
			return err
		}
		if record.Kind != componentipc.KindAck {
			return componentipc.ErrFrameDenied
		}
		if record.Receipt.Stream != stream {
			return componentipc.ErrForeignStream
		}
		if record.Receipt.Sequence <= last || record.Receipt.Sequence > want {
			return componentipc.ErrInvalidSequence
		}
		if err := ring.Confirm(record.Receipt.Sequence); err != nil {
			return err
		}
		last = record.Receipt.Sequence
		if last == want {
			return nil
		}
	}
}
