//go:build linux

// Package componentproc defines a deliberately opt-in supervised Linux process
// boundary. No production eBPF or policy path invokes it yet.
package componentproc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"agent-ebpf-filter/internal/componentipc"
	"agent-ebpf-filter/internal/componentshm"
	"agent-ebpf-filter/internal/componentwire"
)

var (
	ErrPrivilegedWorker = errors.New("component Engine worker must run unprivileged")
	ErrInvalidWorker    = errors.New("invalid component Engine worker configuration")
)

// RunWorker connects an unprivileged Engine observer to its supervising
// Collector. The 32-byte per-run credential arrives only through inherited FD3,
// never argv, the environment or a filesystem secret. It performs no policy
// updates and acknowledges only schema-valid events received in sequence.
func RunWorker(ctx context.Context, socketPath string) error {
	return runWorker(ctx, socketPath, false, uint32(os.Geteuid()))
}

// RunWorkerShared uses inherited FD4/FD5 for the event ring and its notifier.
// The same authenticated UDS carries validation ACKs, never raw event bodies.
func RunWorkerShared(ctx context.Context, socketPath string) error {
	return runWorker(ctx, socketPath, true, uint32(os.Geteuid()))
}

// RunWorkerSharedPrivilegedSupervisor expects a root-owned Collector and an
// explicit non-root Engine. The abstract UDS requires no writable filesystem
// socket or directory and the peer is still verified with Linux SO_PEERCRED.
func RunWorkerSharedPrivilegedSupervisor(ctx context.Context, socketPath string) error {
	if !strings.HasPrefix(socketPath, "@agent-ebpf-engine-") {
		return ErrInvalidWorker
	}
	return runWorker(ctx, socketPath, true, 0)
}

func runWorker(ctx context.Context, socketPath string, shared bool, supervisorUID uint32) error {
	if ctx == nil || !(filepath.IsAbs(socketPath) || strings.HasPrefix(socketPath, "@agent-ebpf-engine-")) {
		return ErrInvalidWorker
	}
	if os.Geteuid() == 0 {
		return ErrPrivilegedWorker
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	secretFD := os.NewFile(3, "component-engine-credential")
	if secretFD == nil {
		return ErrInvalidWorker
	}
	var secret [32]byte
	_, readErr := io.ReadFull(secretFD, secret[:])
	_ = secretFD.Close()
	if readErr != nil {
		clear(secret[:])
		return fmt.Errorf("read inherited component credential: %w", readErr)
	}
	defer clear(secret[:])

	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()
	session, err := componentipc.HandshakeClient(conn, componentipc.ClientConfig{
		Secret: secret[:], Role: componentipc.RoleEngine,
		ExpectedServer: componentipc.RoleCollector,
		VerifyPeer:     componentipc.VerifyUnixPeerUIDs(supervisorUID),
	})
	if err != nil {
		return err
	}
	defer session.Close()
	if shared {
		mem := os.NewFile(4, "component-event-memfd")
		wake := os.NewFile(5, "component-event-wakeup")
		if mem == nil || wake == nil {
			if mem != nil {
				_ = mem.Close()
			}
			if wake != nil {
				_ = wake.Close()
			}
			return ErrInvalidWorker
		}
		ring, err := componentshm.OpenReader(mem, wake)
		if err != nil {
			_ = mem.Close()
			_ = wake.Close()
			return err
		}
		defer ring.Close()
		return consumeShared(ctx, session, ring)
	}

	var receiver *componentipc.SequenceReceiver
	for {
		record, recvErr := session.ReceiveRecordCopy()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) && ctx.Err() == nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return recvErr
		}
		if record.Kind != componentipc.KindEvent {
			return fmt.Errorf("%w: unexpected kind %d", componentipc.ErrFrameDenied, record.Kind)
		}
		// This stage validates and consumes events in a separate process but
		// deliberately does NOT run ML inference, issue alerts or apply policy.
		if _, err := componentwire.DecodeEvent(record.Data.Payload); err != nil {
			return err
		}
		if receiver == nil {
			receiver, err = componentipc.NewSequenceReceiver(record.Data.Stream)
			if err != nil {
				return err
			}
		}
		if err := receiver.Check(record.Data); err != nil {
			return err
		}
		ack, err := receiver.Commit(record.Data)
		if err != nil {
			return err
		}
		if err := session.SendAck(ack); err != nil {
			return err
		}
	}
}

// consumeShared is a read-only Engine observer. Its read cursor is private
// to the Engine and never mutates the sealed shared data mapping. UDS ACKs
// are emitted only after all payloads in the current batch pass validation;
// only the authenticated Collector may recycle those confirmed slots.
func consumeShared(ctx context.Context, session *componentipc.Session, ring *componentshm.Ring) error {
	stream := componentipc.StreamID(ring.Stream())
	if stream == (componentipc.StreamID{}) {
		return componentshm.ErrInvalidRing
	}
	lastAcked := uint64(0)
	for {
		if err := ring.WaitReady(ctx); err != nil {
			if errors.Is(err, componentshm.ErrRingClosed) {
				return nil
			}
			return err
		}
		var lastRead uint64
		for {
			data, err := ring.ReadCopy()
			if errors.Is(err, componentshm.ErrNoRecord) || errors.Is(err, componentshm.ErrRingClosed) {
				break
			}
			if err != nil {
				return err
			}
			if _, err := componentwire.DecodeEvent(data); err != nil {
				return err
			}
			lastRead = ring.Stats().Consumed
		}
		if lastRead > lastAcked {
			if err := session.SendAck(componentipc.Ack{
				Stream: stream, Sequence: lastRead,
			}); err != nil {
				return err
			}
			lastAcked = lastRead
		}
	}
}
