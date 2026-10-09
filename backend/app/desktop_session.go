package app

import (
	"agent-ebpf-filter/udsframe"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
)

const (
	desktopNativeIPCVersion = 2

	desktopFrameEventEnvelope byte = 1 // v1 compatibility
	desktopFrameSystemStats   byte = 2
	desktopFrameEventSummary  byte = 3

	desktopNativeQueueSize    = 1024
	desktopNativeWriteTimeout = 750 * time.Millisecond
)

type desktopProtoFrame struct {
	kind    byte
	message proto.Message
}

var activeDesktopSession atomic.Pointer[desktopSession]

// These explicit arguments survive sudo/pkexec's environment sanitization.
// Only the desktop-launched backend uses them; normal service startup is unchanged.
func ConfigureDesktopFlags(args []string) error {
	flags := flag.NewFlagSet("agent-ebpf-filter", flag.ContinueOnError)
	flags.Bool("ebpf-bootstrap", false, "bootstrap eBPF maps and links") // Existing mode; Main reads os.Args.
	socket := flags.String("desktop-lifetime-socket", "", "desktop lifetime Unix socket")
	apiSocket := flags.String("desktop-api-socket", "", "private Unix socket for desktop API; never opens TCP")
	frontend := flags.String("frontend-dir", "", "absolute frontend asset directory")
	home := flags.String("real-home", "", "unprivileged user's home directory")
	port := flags.Int("desktop-port", 0, "desktop backend port")
	dev := flags.Bool("desktop-dev", false, "desktop development mode (disables auth)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected backend arguments")
	}
	if *apiSocket != "" {
		if *socket == "" || !filepath.IsAbs(*apiSocket) ||
			filepath.Clean(filepath.Dir(*apiSocket)) != filepath.Clean(filepath.Dir(*socket)) ||
			filepath.Clean(*apiSocket) == filepath.Clean(*socket) {
			return fmt.Errorf("desktop API socket must be distinct and in the lifetime socket directory")
		}
		if *port != 0 {
			return fmt.Errorf("desktop API socket and TCP port cannot both be configured")
		}
	}
	for key, value := range map[string]string{
		"AGENT_DESKTOP_LIFETIME_SOCKET": *socket,
		"AGENT_DESKTOP_API_SOCKET":      *apiSocket,
		"AGENT_FRONTEND_DIST":           *frontend,
		"AGENT_REAL_HOME":               *home,
	} {
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			return fmt.Errorf("%s must be an absolute path", key)
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	if *port != 0 {
		if *port < 1 || *port > 65535 {
			return fmt.Errorf("invalid desktop backend port")
		}
		if err := os.Setenv("AGENT_BACKEND_PORT", fmt.Sprint(*port)); err != nil {
			return err
		}
	}
	if *socket != "" {
		mode, disableAuth := "release", "false"
		if *dev {
			mode, disableAuth = "debug", "true"
		}
		if err := os.Setenv("GIN_MODE", mode); err != nil {
			return err
		}
		return os.Setenv("DISABLE_AUTH", disableAuth)
	}
	return nil
}

// EOF from the ordinary-user desktop cancels the privileged backend without
// requiring another password prompt to kill a root PID. Crashes also close the
// connection. The same private socket carries the native low-latency desktop
// stream after the initial token handshake; no reused/system backend receives it.
type desktopSession struct {
	conn      net.Conn
	ctx       context.Context
	cancel    context.CancelFunc
	frames    chan desktopProtoFrame
	startOnce sync.Once
	closeOnce sync.Once
}

func (s *desktopSession) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.cancel()
		_ = s.conn.Close()
	})
}

func (s *desktopSession) publishToken(token string) error {
	if s == nil {
		return nil
	}
	err := json.NewEncoder(s.conn).Encode(struct {
		Token            string `json:"token"`
		NativeIPCVersion int    `json:"nativeIpcVersion"`
	}{
		Token:            token,
		NativeIPCVersion: desktopNativeIPCVersion,
	})
	if err != nil {
		return err
	}
	s.startWriter()
	return nil
}

func (s *desktopSession) startWriter() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		go func() {
			for {
				select {
				case <-s.ctx.Done():
					return
				case frame := <-s.frames:
					if frame.message == nil {
						continue
					}
					payload, err := proto.Marshal(frame.message)
					if err != nil {
						continue
					}
					if err := s.conn.SetWriteDeadline(time.Now().Add(desktopNativeWriteTimeout)); err != nil {
						s.cancel()
						return
					}
					if err := udsframe.WriteTyped(s.conn, frame.kind, payload); err != nil {
						s.cancel()
						return
					}
				}
			}
		}()
	})
}

// publishProto is deliberately non-blocking. The privileged event path must
// never wait for a slow or frozen desktop renderer. Serialization is also kept
// off the producer goroutine and happens in the session writer.
func (s *desktopSession) publishProto(kind byte, message proto.Message) bool {
	if s == nil || message == nil {
		return false
	}
	select {
	case <-s.ctx.Done():
		return false
	default:
	}
	select {
	case s.frames <- desktopProtoFrame{kind: kind, message: message}:
		return true
	default:
		return false
	}
}

func desktopSessionContext(parent context.Context) (context.Context, *desktopSession, error) {
	path := os.Getenv("AGENT_DESKTOP_LIFETIME_SOCKET")
	if path == "" {
		return parent, nil, nil
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, nil, fmt.Errorf("connect desktop lifetime socket: %w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	session := &desktopSession{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
		frames: make(chan desktopProtoFrame, desktopNativeQueueSize),
	}
	go func() {
		_, _ = io.Copy(io.Discard, conn)
		cancel()
	}()
	return ctx, session, nil
}
