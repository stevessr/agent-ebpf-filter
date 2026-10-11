//go:build linux

package componentproc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"agent-ebpf-filter/internal/componentipc"
	"agent-ebpf-filter/internal/componentshm"
)

var ErrInvalidEngineIdentity = errors.New("privileged Collector requires an explicit non-root Engine UID and GID")

// RunEngineSharedAsUID is an explicit, off-by-default privileged launch
// experiment. It NEVER runs an Engine as root and NEVER starts automatically
// from app.Main or Renew. The executable must be root-owned and not writable
// by group/others; supply a separately installed Engine binary.
//
// Linux abstract Unix sockets avoid relaxing the root-owned private directory
// to permit another UID to connect. SO_PEERCRED is checked AFTER the child's
// credential switch and HMAC authenticates the per-session secret on FD3.
// The memfd/eventfd occupy FD4/FD5 and the child maps event data read-only.
func RunEngineSharedAsUID(
	ctx context.Context, executable string, uid, gid uint32, slots, payload int,
	publish func(*componentipc.Session, *componentshm.Ring) error,
) error {
	if ctx == nil || publish == nil || !filepath.IsAbs(executable) {
		return ErrInvalidSupervisor
	}
	if os.Geteuid() != 0 {
		return ErrUnsafeSupervisor
	}
	if uid == 0 || gid == 0 {
		return ErrInvalidEngineIdentity
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(executable)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || stat.Uid != 0 ||
		info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0001 == 0 {
		return ErrInvalidSupervisor
	}
	ring, err := componentshm.Create(slots, payload)
	if err != nil {
		return err
	}
	defer ring.Close()
	memory, wake, err := ring.Files()
	if err != nil {
		return err
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return err
	}
	defer clear(secret[:])
	var socketID [16]byte
	if _, err := rand.Read(socketID[:]); err != nil {
		return err
	}
	abstractName := "@agent-ebpf-engine-" + hex.EncodeToString(socketID[:])
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: abstractName, Net: "unix"})
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := listener.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	credentialReader, credentialWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer credentialReader.Close()
	defer credentialWriter.Close()

	childCtx, stopChild := context.WithCancel(ctx)
	defer stopChild()
	cmd := exec.CommandContext(childCtx, executable,
		"--shared", "--privileged-supervisor", "--socket", abstractName)
	cmd.Dir = "/"
	cmd.Env = []string{"LANG=C"}
	cmd.ExtraFiles = []*os.File{credentialReader, memory, wake}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uid, Gid: gid, Groups: []uint32{}},
		Pdeathsig: syscall.SIGKILL,
	}
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch dropped-UID Engine: %w", err)
	}
	_ = credentialReader.Close()
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	joined := false
	defer func() {
		if !joined {
			stopChild()
			_ = listener.Close()
			<-waitDone
		}
	}()
	if n, writeErr := credentialWriter.Write(secret[:]); writeErr != nil || n != len(secret) {
		return fmt.Errorf("deliver inherited credential: %v (bytes=%d)", writeErr, n)
	}
	_ = credentialWriter.Close()
	stopAccept := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stopAccept()

	accepted := make(chan struct {
		conn *net.UnixConn
		err error
	}, 1)
	go func() {
		conn, acceptErr := listener.AcceptUnix()
		accepted <- struct {
			conn *net.UnixConn
			err error
		}{conn:conn,err:acceptErr}
	}()
	var conn *net.UnixConn
	select {
	case result := <-accepted:
		if result.err != nil {
			return fmt.Errorf("accept Engine UID %d: %w", uid, result.err)
		}
		conn = result.conn
	case childErr := <-waitDone:
		joined = true
		return fmt.Errorf("Engine exited before authentication: %w", childErr)
	case <-ctx.Done():
		return ctx.Err()
	}
	defer conn.Close()
	stopConn := context.AfterFunc(childCtx, func() { _ = conn.Close() })
	defer stopConn()
	session, err := componentipc.HandshakeServer(conn, componentipc.ServerConfig{
		Secret: secret[:], Role: componentipc.RoleCollector,
		AllowedPeers: []componentipc.Role{componentipc.RoleEngine},
		VerifyPeer: componentipc.VerifyUnixPeerUIDs(uid),
	})
	if err != nil {
		return fmt.Errorf("authenticate dropped-UID Engine: %w", err)
	}
	defer session.Close()
	publishErr := publish(session, ring)
	finishErr := ring.Finish()
	if publishErr != nil {
		return publishErr
	}
	if finishErr != nil {
		return finishErr
	}
	select {
	case childErr := <-waitDone:
		joined = true
		return childErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
