package shell

import (
	"testing"
	"time"
)

func TestManagerCloseStopsSessionsAndRejectsNewWork(t *testing.T) {
	manager := NewManager()
	readDone := make(chan struct{})
	close(readDone)
	session := &Session{
		id:       "session-1",
		status:   StatusRunning,
		readDone: readDone,
	}
	manager.sessions[session.id] = session
	notify := manager.Subscribe()

	if err := manager.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !session.closed || session.status != StatusClosed {
		t.Fatalf("session after close = closed:%v status:%q", session.closed, session.status)
	}
	if sessions := manager.List(); len(sessions) != 0 {
		t.Fatalf("sessions after close = %d, want zero", len(sessions))
	}
	select {
	case _, ok := <-notify:
		if ok {
			t.Fatal("manager notification channel remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("manager close did not wake subscribers")
	}
	if _, err := manager.NewSession(CreateRequest{}, Deps{}); err == nil {
		t.Fatal("closed manager accepted a new session")
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}
