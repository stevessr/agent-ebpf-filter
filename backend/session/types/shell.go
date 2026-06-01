package main

import (
	"sync"
	"sync/atomic"
	"time"
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

type ShellSessionInputRequest struct {
	Data string `json:"data"`
}

type shellControlMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

type shellSessionManager struct {
	mu            sync.RWMutex
	nextID        atomic.Uint64
	sessions      map[string]*shellSession
	subscribers   map[chan struct{}]struct{}
	subscribersMu sync.Mutex
}

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
