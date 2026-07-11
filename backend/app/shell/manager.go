package shell

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ---- types ----

// CreateRequest is the JSON body for creating a new shell session.
type CreateRequest struct {
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

// SessionInfo is the public representation of a shell session.
type SessionInfo struct {
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

// InputRequest is the JSON body for sending input to a session.
type InputRequest struct {
	Data string `json:"data"`
}

// ControlMessage is used for resize etc. over WebSocket.
type ControlMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// Manager manages shell sessions.
type Manager struct {
	lifecycleMu   sync.Mutex
	closed        bool
	mu            sync.RWMutex
	nextID        atomic.Uint64
	sessions      map[string]*Session
	subscribers   map[chan struct{}]struct{}
	subscribersMu sync.Mutex
	notifyClosed  bool
}

// NewManager creates a new shell session manager.
func NewManager() *Manager {
	return &Manager{
		sessions:    make(map[string]*Session),
		subscribers: make(map[chan struct{}]struct{}),
	}
}

func (m *Manager) Subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	m.subscribersMu.Lock()
	if m.notifyClosed {
		close(ch)
		m.subscribersMu.Unlock()
		return ch
	}
	m.subscribers[ch] = struct{}{}
	m.subscribersMu.Unlock()
	return ch
}

func (m *Manager) Unsubscribe(ch chan struct{}) {
	m.subscribersMu.Lock()
	delete(m.subscribers, ch)
	m.subscribersMu.Unlock()
}

// Notify wakes all subscribers.
func (m *Manager) Notify() {
	m.subscribersMu.Lock()
	if m.notifyClosed {
		m.subscribersMu.Unlock()
		return
	}
	for ch := range m.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	m.subscribersMu.Unlock()
}

// List returns all sessions sorted by creation time (newest first).
func (m *Manager) List() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		items = append(items, s.Snapshot())
	}
	// sort newest first
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			swap := false
			if items[i].CreatedAt.Equal(items[j].CreatedAt) {
				swap = items[i].ID < items[j].ID
			} else {
				swap = items[i].CreatedAt.Before(items[j].CreatedAt)
			}
			if swap {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	return items
}

// Get returns a session by ID.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// Delete removes a session by ID.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	m.Notify()
	if !ok {
		return fmt.Errorf("shell session not found")
	}
	_ = s.Close()
	return nil
}

// SendInput sends data to a session.
func (m *Manager) SendInput(id string, payload []byte) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("shell session not found")
	}
	return s.WriteInput(payload)
}

// ClearClosed removes all closed sessions.
func (m *Manager) ClearClosed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		s.mu.Lock()
		status := s.status
		closed := s.closed
		s.mu.Unlock()
		if closed || status == StatusExited || status == StatusClosed {
			delete(m.sessions, id)
		}
	}
}

// Close rejects new sessions, terminates every active process, and wakes
// subscribers. It is safe to call more than once.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.lifecycleMu.Lock()
	if m.closed {
		m.lifecycleMu.Unlock()
		return nil
	}
	m.closed = true
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()
	m.lifecycleMu.Unlock()

	m.subscribersMu.Lock()
	if !m.notifyClosed {
		m.notifyClosed = true
		for ch := range m.subscribers {
			close(ch)
			delete(m.subscribers, ch)
		}
	}
	m.subscribersMu.Unlock()

	errs := make([]error, 0, len(sessions))
	for _, session := range sessions {
		if err := session.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
