//go:build !linux

package componentipc

import (
	"errors"
	"net"
)

// The component IPC identity check intentionally fails closed outside Linux.
// The present Agent eBPF collector is Linux-specific; this API cannot be used
// to silently downgrade to unauthenticated TCP or in-memory transports.
func VerifyUnixPeerUIDs(_ ...uint32) Verifier {
	return func(_ net.Conn) error {
		return errors.New("component IPC peer credential checks require Linux")
	}
}
