package shell

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty/v2"
	"github.com/gorilla/websocket"
)

// Deps holds the external dependencies needed by a shell Session.
// The app package provides the concrete implementations.
type Deps struct {
	// EmitEvent sends a pb.Event to the frontend/archive pipeline.
	EmitEvent func(event *pb.Event)
	// DropPrivileges configures cmd.SysProcAttr to drop to the original invoking user.
	DropPrivileges func(cmd *exec.Cmd)
}

// Session is a single PTY-based shell session.
type Session struct {
	mu sync.Mutex

	id        string
	label     string
	kind      string
	shellReq  string
	shellPath string
	command   string
	args      []string
	workDir   string
	createdAt time.Time
	updatedAt time.Time
	status    string
	lastError string
	pid       int

	cmd      *exec.Cmd
	ptmx     *os.File
	conn     *websocket.Conn
	attached bool
	closed   bool

	backlog      []byte
	backlogLimit int
	writeMu      sync.Mutex
	readDone     chan struct{}
	waitOnce     sync.Once
	waitErr      error

	onChange func()
	deps     Deps
}

// NewSession creates a shell session, starts the PTY, and begins the read loop.
// It appends the session to the manager's internal map and notifies subscribers.
func (m *Manager) NewSession(req CreateRequest, deps Deps) (*SessionInfo, error) {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.closed {
		return nil, fmt.Errorf("shell session manager is closed")
	}

	shellReq := stringsTrimToDefault(req.Shell, "auto")
	label := strings.TrimSpace(req.Label)
	commandReq := strings.TrimSpace(req.Command)
	launchArgs := append([]string(nil), req.Args...)
	kind := normalizeShellSessionKind(req.Kind)
	if kind == KindShell {
		shellReqLower := strings.ToLower(shellReq)
		commandReqLower := strings.ToLower(commandReq)
		switch {
		case shellReqLower == KindTmux || commandReqLower == KindTmux:
			kind = KindTmux
		case shellReqLower == KindWrapper || commandReqLower == "agent-wrapper":
			kind = KindWrapper
		case strings.Contains(shellReqLower, "python") || strings.Contains(shellReqLower, "node") ||
			strings.Contains(commandReqLower, "python") || strings.Contains(commandReqLower, "node"):
			kind = KindScript
		}
	}
	launchReq := shellReq
	if commandReq != "" {
		launchReq = commandReq
	}
	if label == "" {
		label = launchReq
	}

	var launchPath string
	if kind == KindWrapper || shellReq == KindWrapper || commandReq == "agent-wrapper" || launchReq == "agent-wrapper" {
		launchPath = platform.ResolveWrapperPath()
	} else {
		launchPath = platform.ResolveShellPath(launchReq)
	}
	if launchPath == "" {
		return nil, fmt.Errorf("launcher not found")
	}

	workDir := platform.ResolveShellWorkDir()
	if req.WorkDir != "" {
		if info, err := os.Stat(req.WorkDir); err == nil && info.IsDir() {
			workDir = req.WorkDir
		} else {
			return nil, fmt.Errorf("invalid working directory: %s", req.WorkDir)
		}
	}

	cols := req.Cols
	if cols <= 0 {
		cols = 100
	}
	rows := req.Rows
	if rows <= 0 {
		rows = 32
	}

	cmd := exec.Command(launchPath, launchArgs...)
	cmd.Dir = workDir
	cmd.Env = platform.SetEnvValue(os.Environ(), "TERM", "xterm-256color")

	// Disable fish shell's query-terminal feature to prevent 10s wait warnings
	ff := os.Getenv("fish_features")
	if ff == "" {
		ff = "no-query-term"
	} else if !strings.Contains(ff, "no-query-term") {
		ff = ff + ",no-query-term"
	}
	cmd.Env = platform.SetEnvValue(cmd.Env, "fish_features", ff)
	for key, value := range req.Env {
		if strings.TrimSpace(key) == "" {
			continue
		}
		cmd.Env = platform.SetEnvValue(cmd.Env, key, value)
	}

	if deps.DropPrivileges != nil {
		deps.DropPrivileges(cmd)
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &Session{
		id:           fmt.Sprintf("%d", m.nextID.Add(1)),
		label:        label,
		kind:         kind,
		shellReq:     shellReq,
		shellPath:    launchPath,
		command:      commandReq,
		args:         append([]string(nil), launchArgs...),
		workDir:      workDir,
		createdAt:    now,
		updatedAt:    now,
		status:       StatusRunning,
		pid:          cmd.Process.Pid,
		cmd:          cmd,
		ptmx:         ptmx,
		readDone:     make(chan struct{}),
		backlogLimit: BacklogLimit,
		onChange:     func() { m.Notify() },
		deps:         deps,
	}

	m.mu.Lock()
	m.sessions[session.id] = session
	m.mu.Unlock()

	m.Notify()

	go session.readLoop(ptmx)

	info := session.Snapshot()
	return &info, nil
}

func (s *Session) readLoop(ptmx *os.File) {
	defer close(s.readDone)
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			s.forwardOutput(bytes.Clone(buf[:n]))
		}
		if err != nil {
			s.finishRead(err)
			return
		}
	}
}

func (s *Session) emitStdioEvent(stream string, payload []byte) {
	if s == nil || len(payload) == 0 || s.deps.EmitEvent == nil {
		return
	}
	fd := "stdout"
	if stream == "stdin" {
		fd = "stdin"
	}
	event := &pb.Event{
		Pid:           uint32(maxInt(s.pid, 0)),
		Type:          "stdio",
		EventType:     pb.EventType_STDIO,
		Tag:           "Shell Session",
		Comm:          platform.FirstNonEmpty(s.label, s.kind, "shell"),
		Path:          stream,
		Bytes:         uint64(len(payload)),
		ExtraInfo:     fmt.Sprintf("session_id=%s stream=%s fd=%s size=%d", s.id, stream, fd, len(payload)),
		SchemaVersion: PB_SchemaVersion,
		Cwd:           s.workDir,
	}
	s.deps.EmitEvent(event)
}

func (s *Session) forwardOutput(payload []byte) {
	if len(payload) == 0 {
		return
	}

	s.emitStdioEvent("stdout", payload)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.appendBacklogLocked(payload)
	conn := s.conn
	s.updatedAt = time.Now()
	s.mu.Unlock()

	if conn == nil {
		return
	}

	s.writeMu.Lock()
	err := conn.WriteMessage(websocket.BinaryMessage, payload)
	s.writeMu.Unlock()
	if err != nil {
		s.Detach(conn)
	}
}

func (s *Session) finishRead(readErr error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		s.status = StatusError
		s.lastError = readErr.Error()
	} else {
		s.status = StatusExited
	}
	s.updatedAt = time.Now()
	conn := s.conn
	s.conn = nil
	s.attached = false
	ptmx := s.ptmx
	s.ptmx = nil
	s.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	if ptmx != nil {
		_ = ptmx.Close()
	}
	_ = s.waitProcess()
	if s.onChange != nil {
		s.onChange()
	}
}

// Attach attaches a WebSocket connection to the session, sending the backlog first.
func (s *Session) Attach(conn *websocket.Conn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("shell session has been closed")
	}
	if s.status != StatusRunning {
		return fmt.Errorf("shell session is not running")
	}
	if s.conn != nil {
		return fmt.Errorf("shell session is already attached")
	}
	s.conn = conn
	s.attached = true
	s.updatedAt = time.Now()
	if len(s.backlog) > 0 {
		s.writeMu.Lock()
		err := conn.WriteMessage(websocket.BinaryMessage, bytes.Clone(s.backlog))
		s.writeMu.Unlock()
		if err != nil {
			s.conn = nil
			s.attached = false
			return fmt.Errorf("backlog write failed: %w", err)
		}
	}
	return nil
}

// Detach removes a WebSocket connection from the session.
func (s *Session) Detach(conn *websocket.Conn) {
	s.mu.Lock()
	if s.conn == conn {
		s.conn = nil
		s.attached = false
		s.updatedAt = time.Now()
	}
	s.mu.Unlock()
	_ = conn.Close()
}

// WriteInput writes data to the PTY.
func (s *Session) WriteInput(payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("shell session has been closed")
	}
	if s.ptmx == nil {
		return fmt.Errorf("shell session PTY is unavailable")
	}
	_, err := s.ptmx.Write(payload)
	if err == nil {
		s.emitStdioEvent("stdin", payload)
	}
	return err
}

// Resize resizes the PTY.
func (s *Session) Resize(cols, rows int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("shell session has been closed")
	}
	if s.ptmx == nil {
		return fmt.Errorf("shell session PTY is unavailable")
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

// Close terminates the session.
func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.status = StatusClosed
	s.updatedAt = time.Now()
	conn := s.conn
	s.conn = nil
	s.attached = false
	ptmx := s.ptmx
	s.ptmx = nil
	cmd := s.cmd
	s.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	if ptmx != nil {
		_ = ptmx.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = s.waitProcess()
	if s.readDone != nil {
		<-s.readDone
	}
	return nil
}

func (s *Session) waitProcess() error {
	if s == nil {
		return nil
	}
	s.waitOnce.Do(func() {
		if s.cmd != nil {
			s.waitErr = s.cmd.Wait()
		}
	})
	return s.waitErr
}

// Snapshot returns the public info for this session.
func (s *Session) Snapshot() SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SessionInfo{
		ID:        s.id,
		Label:     s.label,
		Kind:      s.kind,
		Shell:     s.shellReq,
		ShellPath: s.shellPath,
		Command:   s.command,
		Args:      append([]string(nil), s.args...),
		WorkDir:   s.workDir,
		PID:       s.pid,
		Status:    s.status,
		Attached:  s.attached,
		CreatedAt: s.createdAt,
		UpdatedAt: s.updatedAt,
		LastError: s.lastError,
	}
}

func (s *Session) appendBacklogLocked(payload []byte) {
	if len(payload) == 0 {
		return
	}
	if s.backlogLimit <= 0 {
		s.backlog = append(s.backlog, payload...)
		return
	}
	if len(payload) >= s.backlogLimit {
		s.backlog = append(s.backlog[:0], payload[len(payload)-s.backlogLimit:]...)
		return
	}
	overflow := len(s.backlog) + len(payload) - s.backlogLimit
	if overflow > 0 {
		if overflow >= len(s.backlog) {
			s.backlog = append(s.backlog[:0], payload...)
			return
		}
		s.backlog = append(bytes.Clone(s.backlog[overflow:]), payload...)
		return
	}
	s.backlog = append(s.backlog, payload...)
}
