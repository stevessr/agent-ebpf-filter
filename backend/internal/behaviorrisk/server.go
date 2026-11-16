package behaviorrisk

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"agent-ebpf-filter/internal/behaviorfeatures"
	"agent-ebpf-filter/internal/strictjson"
)

const MaxConnections = 4
const ConnectionBudget = 4 * 1024 * 1024
const ConnectionLifetime = 10 * time.Second

// ListenPrivate requires an existing owner-only directory. It never removes an
// existing socket/file to bind, and never listens on TCP.
func ListenPrivate(socket string) (*net.UnixListener, error) {
	if !filepath.IsAbs(socket) {
		return nil, errors.New("absolute private socket path required")
	}
	dir, e := os.Lstat(filepath.Dir(socket))
	if e != nil || !dir.IsDir() || dir.Mode().Perm() != 0700 {
		return nil, errors.New("socket directory must be private (0700)")
	}
	stat, ok := dir.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return nil, errors.New("socket directory owner mismatch")
	}
	l, e := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if e != nil {
		if op, ok := e.(*net.OpError); ok {
			return nil, fmt.Errorf("socket bind failed: %w", op.Err)
		}
		return nil, errors.New("socket bind failed")
	}
	if e = os.Chmod(socket, 0600); e != nil {
		l.Close()
		return nil, errors.New("socket permissions failed")
	}
	return l, nil
}

type ServerStats struct {
	Accepted  uint64 `json:"accepted"`
	Rejected  uint64 `json:"rejected"`
	Records   uint64 `json:"records"`
	Abstained uint64 `json:"abstained"`
}
type Server struct {
	Manager                                *Manager
	accepted, rejected, records, abstained atomic.Uint64
}

func (s *Server) Stats() ServerStats {
	return ServerStats{s.accepted.Load(), s.rejected.Load(), s.records.Load(), s.abstained.Load()}
}

// Serve applies finite connection/byte/time limits and shuts down all clients
// on cancellation. Counter-only diagnostics contain no requests or identifiers.
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	if s.Manager == nil {
		return errors.New("model manager required")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	slots := make(chan struct{}, MaxConnections)
	var wg sync.WaitGroup
	var mu sync.Mutex
	active := map[net.Conn]bool{}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		l.Close()
		mu.Lock()
		for c := range active {
			c.Close()
		}
		mu.Unlock()
	}()
	defer func() { cancel(); <-shutdownDone; wg.Wait() }()
	for {
		c, e := l.Accept()
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errors.New("socket accept failed")
		}
		select {
		case slots <- struct{}{}:
			mu.Lock()
			if ctx.Err() != nil {
				mu.Unlock()
				c.Close()
				<-slots
				return nil
			}
			active[c] = true
			mu.Unlock()
			s.accepted.Add(1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { mu.Lock(); delete(active, c); mu.Unlock(); c.Close(); <-slots }()
				s.handle(c)
			}()
		default:
			s.rejected.Add(1)
			c.Close()
		}
	}
}
func (s *Server) handle(c net.Conn) {
	if c.SetDeadline(time.Now().Add(ConnectionLifetime)) != nil {
		return
	}
	scanner := bufio.NewScanner(io.LimitReader(c, ConnectionBudget))
	scanner.Buffer(make([]byte, 4096), 512*1024)
	enc := json.NewEncoder(c)
	for scanner.Scan() {
		s.records.Add(1)
		var input behaviorfeatures.Input
		r := Result{Mode: "shadow_only", Status: "abstain"}
		if strictjson.Decode(scanner.Bytes(), &input) == nil {
			r = s.Manager.Score(input)
		}
		if r.Score == nil {
			s.abstained.Add(1)
		}
		if enc.Encode(r) != nil {
			return
		}
	}
}
