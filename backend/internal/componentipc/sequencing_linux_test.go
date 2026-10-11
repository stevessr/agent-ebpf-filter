//go:build linux

package componentipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This exercises the sequenced codec and explicit receipt through real Unix
// sockets and SO_PEERCRED on both peers, without a privileged kernel fixture.
func TestSequencedReceiptOverAuthenticatedUnixSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "sequenced.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	uid := uint32(os.Getuid())
	stream := fixedStream(t)
	result := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		server, err := HandshakeServer(conn, ServerConfig{
			Secret: testSecret(), Role: RoleEngine,
			AllowedPeers: []Role{RoleCollector},
			VerifyPeer:   VerifyUnixPeerUIDs(uid),
		})
		if err != nil {
			result <- err
			return
		}
		defer server.Close()
		if err := server.conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			result <- err
			return
		}
		message, err := server.ReceiveRecordCopy()
		if err != nil {
			result <- err
			return
		}
		if message.Kind != KindEvent || message.Data.Stream != stream ||
			message.Data.Sequence != 1 || string(message.Data.Payload) != "uds-event" {
			result <- fmt.Errorf("unexpected wire data: %+v", message)
			return
		}
		receiver, err := NewSequenceReceiver(stream)
		if err != nil {
			result <- err
			return
		}
		if err := receiver.Check(message.Data); err != nil {
			result <- err
			return
		}
		// The simulated application has validated the payload before Commit.
		receipt, err := receiver.Commit(message.Data)
		if err != nil {
			result <- err
			return
		}
		result <- server.SendAck(receipt)
	}()

	raw, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	client, err := HandshakeClient(raw, ClientConfig{
		Secret: testSecret(), Role: RoleCollector,
		ExpectedServer: RoleEngine, VerifyPeer: VerifyUnixPeerUIDs(uid),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.SendSequenced(KindEvent, Sequenced{
		Stream: stream, Sequence: 1, Payload: []byte("uds-event"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	received, err := client.ReceiveRecordCopy()
	if err != nil {
		t.Fatal(err)
	}
	if received.Kind != KindAck || received.Receipt.Stream != stream || received.Receipt.Sequence != 1 {
		t.Fatalf("unexpected Unix socket receipt: %+v", received)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
