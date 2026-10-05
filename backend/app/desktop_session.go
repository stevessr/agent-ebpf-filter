package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

// These explicit arguments survive sudo/pkexec's environment sanitization.
// Only the desktop-launched backend uses them; normal service startup is unchanged.
func ConfigureDesktopFlags(args []string) error {
	flags := flag.NewFlagSet("agent-ebpf-filter", flag.ContinueOnError)
	flags.Bool("ebpf-bootstrap", false, "bootstrap eBPF maps and links") // Existing mode; Main reads os.Args.
	socket := flags.String("desktop-lifetime-socket", "", "desktop lifetime Unix socket")
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
	for key, value := range map[string]string{
		"AGENT_DESKTOP_LIFETIME_SOCKET": *socket,
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
// connection. No reused/system backend receives this lifetime socket.
type desktopSession struct {
	conn   net.Conn
	cancel context.CancelFunc
}

func (s *desktopSession) Close() {
	if s != nil {
		s.cancel()
		_ = s.conn.Close()
	}
}

func (s *desktopSession) publishToken(token string) error {
	if s == nil {
		return nil
	}
	return json.NewEncoder(s.conn).Encode(struct {
		Token string `json:"token"`
	}{token})
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
	go func() {
		_, _ = io.Copy(io.Discard, conn)
		cancel()
	}()
	return ctx, &desktopSession{conn: conn, cancel: cancel}, nil
}
