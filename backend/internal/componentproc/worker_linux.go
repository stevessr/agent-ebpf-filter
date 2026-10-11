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

	"agent-ebpf-filter/internal/componentipc"
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
	if ctx == nil || !filepath.IsAbs(socketPath) {
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
		VerifyPeer: componentipc.VerifyUnixPeerUIDs(uint32(os.Geteuid())),
	})
	if err != nil {
		return err
	}
	defer session.Close()

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
		if _, err := componentwire.DecodeEventEnvelope(record.Data.Payload); err != nil {
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
