package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty/v2"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	shellSessionStatusRunning = "running"
	shellSessionStatusExited  = "exited"
	shellSessionStatusClosed  = "closed"
	shellSessionStatusError   = "error"
)

const shellSessionBacklogLimit = 1 << 20

const (
	shellSessionKindShell   = "shell"
	shellSessionKindTmux    = "tmux"
	shellSessionKindScript  = "script"
	shellSessionKindWrapper = "wrapper"
)

type ShellSessionCreateRequest struct {
	Shell   string            `json:"shell"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Label   string            `json:"label,omitempty"`
	WorkDir string            `json:"workDir,omitempty"`
	Cols    int               `json:"cols,omitempty"`
	Rows    int               `json:"rows,omitempty"`
	Kind    string            `json:"kind,omitempty"`
}

type ShellSessionInfo struct {
	ID        string    `json:"id"`
	Label     string    `json:"label,omitempty"`
	Kind      string    `json:"kind"`
	Shell     string    `json:"shell"`
	ShellPath string    `json:"shellPath"`
	Command   string    `json:"command,omitempty"`
	Args      []string  `json:"args,omitempty"`
	WorkDir   string    `json:"workDir"`
	PID       int       `json:"pid"`
	Status    string    `json:"status"`
	Attached  bool      `json:"attached"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	LastError string    `json:"lastError,omitempty"`
}

type shellSession struct {
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

	onChange func()
}

type ShellSessionInputRequest struct {
	Data string `json:"data"`
}

type shellSessionManager struct {
	mu            sync.RWMutex
	nextID        atomic.Uint64
	sessions      map[string]*shellSession
	subscribers   map[chan struct{}]struct{}
	subscribersMu sync.Mutex
}

var shellSessions = newShellSessionManager()

func newShellSessionManager() *shellSessionManager {
	return &shellSessionManager{
		sessions:    make(map[string]*shellSession),
		subscribers: make(map[chan struct{}]struct{}),
	}
}

func (m *shellSessionManager) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	m.subscribersMu.Lock()
	m.subscribers[ch] = struct{}{}
	m.subscribersMu.Unlock()
	return ch
}

func (m *shellSessionManager) unsubscribe(ch chan struct{}) {
	m.subscribersMu.Lock()
	delete(m.subscribers, ch)
	m.subscribersMu.Unlock()
}

func (m *shellSessionManager) notify() {
	m.subscribersMu.Lock()
	for ch := range m.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	m.subscribersMu.Unlock()
}

func (m *shellSessionManager) Create(req ShellSessionCreateRequest) (*ShellSessionInfo, error) {
	shellReq := stringsTrimToDefault(req.Shell, "auto")
	label := strings.TrimSpace(req.Label)
	commandReq := strings.TrimSpace(req.Command)
	launchArgs := append([]string(nil), req.Args...)
	kind := normalizeShellSessionKind(req.Kind)
	if kind == shellSessionKindShell {
		shellReqLower := strings.ToLower(shellReq)
		commandReqLower := strings.ToLower(commandReq)
		switch {
		case shellReqLower == shellSessionKindTmux || commandReqLower == shellSessionKindTmux:
			kind = shellSessionKindTmux
		case shellReqLower == shellSessionKindWrapper || commandReqLower == "agent-wrapper":
			kind = shellSessionKindWrapper
		case strings.Contains(shellReqLower, "python") || strings.Contains(shellReqLower, "node") ||
			strings.Contains(commandReqLower, "python") || strings.Contains(commandReqLower, "node"):
			kind = shellSessionKindScript
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
	if kind == shellSessionKindWrapper || shellReq == shellSessionKindWrapper || commandReq == "agent-wrapper" || launchReq == "agent-wrapper" {
		launchPath = resolveWrapperPath()
	} else {
		launchPath = resolveShellPath(launchReq)
	}
	if launchPath == "" {
		return nil, fmt.Errorf("launcher not found")
	}

	workDir := resolveShellWorkDir()
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
	cmd.Env = setEnvValue(os.Environ(), "TERM", "xterm-256color")

	// Disable fish shell's query-terminal feature to prevent 10s wait warnings
	ff := os.Getenv("fish_features")
	if ff == "" {
		ff = "no-query-term"
	} else if !strings.Contains(ff, "no-query-term") {
		ff = ff + ",no-query-term"
	}
	cmd.Env = setEnvValue(cmd.Env, "fish_features", ff)
	for key, value := range req.Env {
		if strings.TrimSpace(key) == "" {
			continue
		}
		cmd.Env = setEnvValue(cmd.Env, key, value)
	}

	dropPrivileges(cmd)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &shellSession{
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
		status:       shellSessionStatusRunning,
		pid:          cmd.Process.Pid,
		cmd:          cmd,
		ptmx:         ptmx,
		backlogLimit: shellSessionBacklogLimit,
		onChange:     func() { m.notify() },
	}

	m.mu.Lock()
	m.sessions[session.id] = session
	m.mu.Unlock()

	m.notify()

	go session.readLoop()

	info := session.snapshot()
	return &info, nil
}

func (m *shellSessionManager) List() []ShellSessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]ShellSessionInfo, 0, len(m.sessions))
	for _, session := range m.sessions {
		items = append(items, session.snapshot())
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items
}

func (m *shellSessionManager) Get(id string) (*shellSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[id]
	return session, ok
}

func (m *shellSessionManager) Delete(id string) error {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	m.notify()
	if !ok {
		return fmt.Errorf("shell session not found")
	}
	_ = session.Close()
	return nil
}

func (m *shellSessionManager) SendInput(id string, payload []byte) error {
	session, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("shell session not found")
	}
	return session.WriteInput(payload)
}

func (m *shellSessionManager) ClearClosed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, session := range m.sessions {
		session.mu.Lock()
		status := session.status
		closed := session.closed
		session.mu.Unlock()
		if closed || status == shellSessionStatusExited || status == shellSessionStatusClosed {
			delete(m.sessions, id)
		}
	}
}

func (m *shellSessionManager) AttachWS(c *gin.Context) {
	sessionID := stringsTrimToDefault(c.Query("session_id"), "")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	session, ok := m.Get(sessionID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "shell session not found"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	backlog, err := session.Attach(conn)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(err.Error()))
		_ = conn.Close()
		return
	}
	defer session.Detach(conn)

	if len(backlog) > 0 {
		session.writeMu.Lock()
		if err := conn.WriteMessage(websocket.BinaryMessage, backlog); err != nil {
			session.writeMu.Unlock()
			return
		}
		session.writeMu.Unlock()
	}

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		switch messageType {
		case websocket.BinaryMessage:
			if len(data) == 0 {
				continue
			}
			if err := session.WriteInput(data); err != nil {
				return
			}
		case websocket.TextMessage:
			var ctrl shellControlMessage
			if err := json.Unmarshal(data, &ctrl); err == nil && ctrl.Type == "resize" {
				if ctrl.Cols > 0 && ctrl.Rows > 0 {
					_ = session.Resize(ctrl.Cols, ctrl.Rows)
				}
				continue
			}
			if err := session.WriteInput(data); err != nil {
				return
			}
		}
	}
}

func (s *shellSession) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			s.forwardOutput(bytes.Clone(buf[:n]))
		}
		if err != nil {
			s.finishRead(err)
			return
		}
	}
}

func (s *shellSession) forwardOutput(payload []byte) {
	if len(payload) == 0 {
		return
	}

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

func (s *shellSession) finishRead(readErr error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		s.status = shellSessionStatusError
		s.lastError = readErr.Error()
	} else {
		s.status = shellSessionStatusExited
	}
	s.updatedAt = time.Now()
	conn := s.conn
	s.conn = nil
	s.attached = false
	s.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	if s.onChange != nil {
		s.onChange()
	}
}

func (s *shellSession) Attach(conn *websocket.Conn) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("shell session has been closed")
	}
	if s.status != shellSessionStatusRunning {
		return nil, fmt.Errorf("shell session is not running")
	}
	if s.conn != nil {
		return nil, fmt.Errorf("shell session is already attached")
	}
	s.conn = conn
	s.attached = true
	s.updatedAt = time.Now()
	return bytes.Clone(s.backlog), nil
}

func (s *shellSession) Detach(conn *websocket.Conn) {
	s.mu.Lock()
	if s.conn == conn {
		s.conn = nil
		s.attached = false
		s.updatedAt = time.Now()
	}
	s.mu.Unlock()
	_ = conn.Close()
}

func (s *shellSession) WriteInput(payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("shell session has been closed")
	}
	if s.ptmx == nil {
		return fmt.Errorf("shell session PTY is unavailable")
	}
	_, err := s.ptmx.Write(payload)
	return err
}

func (s *shellSession) Resize(cols, rows int) error {
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

func (s *shellSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.status = shellSessionStatusClosed
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
	if cmd != nil {
		_ = cmd.Wait()
	}
	return nil
}

func (s *shellSession) snapshot() ShellSessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := ShellSessionInfo{
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
	return info
}

func (s *shellSession) appendBacklogLocked(payload []byte) {
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

func handleCreateShellSession(c *gin.Context) {
	var req ShellSessionCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	session, err := shellSessions.Create(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, session)
}

func handleListShellSessions(c *gin.Context) {
	c.JSON(http.StatusOK, shellSessions.List())
}

func handleDeleteShellSession(c *gin.Context) {
	if err := shellSessions.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func handleSendShellSessionInput(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id is required"})
		return
	}

	var req ShellSessionInputRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data := []byte(req.Data)
	if len(data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "data is required"})
		return
	}

	if err := shellSessions.SendInput(sessionID, data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func stringsTrimToDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func normalizeShellSessionKind(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case shellSessionKindTmux:
		return shellSessionKindTmux
	case shellSessionKindScript:
		return shellSessionKindScript
	case shellSessionKindWrapper:
		return shellSessionKindWrapper
	case "", shellSessionKindShell:
		return shellSessionKindShell
	default:
		return shellSessionKindShell
	}
}
