//go:build linux

package componentproc

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/internal/componentipc"
	"agent-ebpf-filter/internal/componentwire"
)

func testWorkerBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "component-engine-worker")
	cmd := exec.Command("go", "build", "-o", path, "./cmd/component-engine-worker")
	cmd.Dir = "../.."
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile split Engine worker: %v\n%s", err, output)
	}
	return path
}

func testEnvelope(id string) componentwire.Event {
	return componentwire.Event{
		SchemaVersion: componentwire.EnvelopeVersion,
		EventID:       id,
		Source:        "kernel",
		TimestampNS:   12345,
		Type:          "exec",
	}
}

func TestRunEngineUsesActualChildProcessAndAcknowledgesEvents(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root must not run a same-UID Engine")
	}
	exe := testWorkerBinary(t)
	ctx, stop := context.WithTimeout(context.Background(), 12*time.Second)
	defer stop()
	err := RunEngine(ctx, exe, func(session *componentipc.Session) error {
		if session.LocalRole() != componentipc.RoleCollector ||
			session.PeerRole() != componentipc.RoleEngine {
			t.Errorf("unexpected authenticated roles %v -> %v", session.LocalRole(), session.PeerRole())
			return componentipc.ErrRoleDenied
		}
		stream, err := componentipc.NewStreamID()
		if err != nil {
			return err
		}
		for sequence, id := range []string{"one", "two"} {
			if err := SendEvent(session, stream, uint64(sequence+1), testEnvelope(id)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("split Engine did not finish cleanly: %v", err)
	}
}

func TestRunEngineRefusesMalformedEventsWithoutAck(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root must not run a same-UID Engine")
	}
	exe := testWorkerBinary(t)
	ctx, stop := context.WithTimeout(context.Background(), 12*time.Second)
	defer stop()
	err := RunEngine(ctx, exe, func(session *componentipc.Session) error {
		stream, err := componentipc.NewStreamID()
		if err != nil {
			return err
		}
		if err := session.SendSequenced(componentipc.KindEvent, componentipc.Sequenced{
			Stream: stream, Sequence: 1, Payload: []byte("invalid-json-projection"),
		}); err != nil {
			return err
		}
		if _, err := session.ReceiveRecordCopy(); err == nil {
			return errors.New("malformed payload was acknowledged")
		}
		// The worker must fail closed and exit nonzero. Returning nil from the
		// callback lets RunEngine report the real child exit code.
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "exit status") {
		t.Fatalf("invalid payload did not terminate child: %v", err)
	}
}

func TestRunEngineRejectsInvalidInput(t *testing.T) {
	if err := RunEngine(nil, "/bin/false", func(*componentipc.Session) error { return nil }); !errors.Is(err, ErrInvalidSupervisor) {
		t.Fatalf("nil context: %v", err)
	}
	if err := RunEngine(context.Background(), "relative/binary", func(*componentipc.Session) error { return nil }); !errors.Is(err, ErrInvalidSupervisor) {
		t.Fatalf("relative executable: %v", err)
	}
	if err := RunEngine(context.Background(), "/bin/false", nil); !errors.Is(err, ErrInvalidSupervisor) {
		t.Fatalf("missing publisher: %v", err)
	}
	if os.Geteuid() == 0 {
		if err := RunEngine(context.Background(), "/bin/false", func(*componentipc.Session) error { return nil }); !errors.Is(err, ErrUnsafeSupervisor) {
			t.Fatalf("root supervisor unexpectedly allowed: %v", err)
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RunEngine(ctx, "/bin/false", func(*componentipc.Session) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled supervisor started child: %v", err)
	}
}

func TestWorkerRejectsMissingCredentials(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root worker guard checked in supervisor tests")
	}
	if err := RunWorker(context.Background(), "relative/socket"); !errors.Is(err, ErrInvalidWorker) {
		t.Fatalf("relative socket accepted: %v", err)
	}
	if err := RunWorker(nil, "/tmp/never-connect"); !errors.Is(err, ErrInvalidWorker) {
		t.Fatalf("nil worker context accepted: %v", err)
	}
}

func TestSendEventValidatesProjectionBeforeWriting(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	secret := []byte("0123456789abcdef0123456789abcdef")
	verifier := func(net.Conn) error { return nil }
	type result struct {
		session *componentipc.Session
		err error
	}
	accepted := make(chan result, 1)
	go func() {
		session, err := componentipc.HandshakeServer(serverConn, componentipc.ServerConfig{
			Secret: secret, Role: componentipc.RoleEngine,
			AllowedPeers: []componentipc.Role{componentipc.RoleCollector},
			VerifyPeer: verifier,
		})
		accepted <- result{session: session, err: err}
	}()
	client, err := componentipc.HandshakeClient(clientConn, componentipc.ClientConfig{
		Secret: secret, Role: componentipc.RoleCollector,
		ExpectedServer: componentipc.RoleEngine, VerifyPeer: verifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	peer := <-accepted
	if peer.err != nil {
		t.Fatal(peer.err)
	}
	defer peer.session.Close()
	stream, err := componentipc.NewStreamID()
	if err != nil {
		t.Fatal(err)
	}
	bad := testEnvelope("valid")
	bad.SchemaVersion = "wrong-version"
	if err := SendEvent(client, stream, 1, bad); !errors.Is(err, componentwire.ErrInvalidEnvelope) {
		t.Fatalf("invalid projection accepted: %v", err)
	}
	if err := SendEvent(nil, stream, 1, testEnvelope("ok")); !errors.Is(err, componentipc.ErrInvalidConfig) {
		t.Fatalf("nil session accepted: %v", err)
	}
}
