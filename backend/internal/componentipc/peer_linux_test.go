//go:build linux

package componentipc

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestSO_PEERCREDChecksExplicitUserID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peer.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	server, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	uid := uint32(os.Getuid())
	if err := VerifyUnixPeerUIDs(uid)(server); err != nil {
		t.Fatalf("expected current uid: %v", err)
	}
	if err := VerifyUnixPeerUIDs(uid)(conn); err != nil {
		t.Fatalf("client peer uid mismatch: %v", err)
	}
	if err := VerifyUnixPeerUIDs(uid+10000)(server); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("unexpected uid allowed: %v", err)
	}
	if err := VerifyUnixPeerUIDs()(server); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("empty uid allowlist allowed: %v", err)
	}
	if err := VerifyUnixPeerUIDs(uid)(nil); err == nil {
		t.Fatal("non-UDS peer accepted")
	}
}
