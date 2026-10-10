//go:build linux

package componentipc

import (
	"errors"
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// VerifyUnixPeerUIDs rejects any non-Unix connection and verifies SO_PEERCRED
// against an explicit allowlist. Empty allowlists fail closed. Root UID is NOT
// implicitly trusted. Invoke it on both server and client handshake paths.
func VerifyUnixPeerUIDs(allowed ...uint32) Verifier {
	uids := make(map[uint32]struct{}, len(allowed))
	for _, uid := range allowed {
		uids[uid] = struct{}{}
	}
	return func(conn net.Conn) error {
		if len(uids) == 0 {
			return ErrInvalidConfig
		}
		unixConn, ok := conn.(*net.UnixConn)
		if !ok || unixConn == nil {
			return errors.New("component IPC requires a Unix-domain connection")
		}
		raw, err := unixConn.SyscallConn()
		if err != nil {
			return err
		}
		var credential *unix.Ucred
		var peerErr error
		if err := raw.Control(func(fd uintptr) {
			credential, peerErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		}); err != nil {
			return err
		}
		if peerErr != nil {
			return peerErr
		}
		if credential == nil {
			return ErrAuthentication
		}
		if _, ok := uids[credential.Uid]; !ok {
			return fmt.Errorf("%w: unexpected peer uid %d", ErrAuthentication, credential.Uid)
		}
		return nil
	}
}
