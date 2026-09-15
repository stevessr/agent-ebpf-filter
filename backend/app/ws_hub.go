package app

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	protoClientQueueSize    = 16
	protoClientWriteTimeout = 2 * time.Second
	protoClientPingPeriod   = 30 * time.Second
)

type protoBroadcastClient interface {
	WriteMessage(messageType int, data []byte) error
	SetWriteDeadline(deadline time.Time) error
	Close() error
}

type protoClientState struct {
	conn      protoBroadcastClient
	queue     chan []byte
	done      chan struct{}
	closeOnce sync.Once
	dead      atomic.Bool
}

func newProtoClientState(conn protoBroadcastClient) *protoClientState {
	return &protoClientState{
		conn:  conn,
		queue: make(chan []byte, protoClientQueueSize),
		done:  make(chan struct{}),
	}
}

func (state *protoClientState) enqueue(data []byte) bool {
	if state == nil || state.dead.Load() {
		return false
	}
	select {
	case state.queue <- data:
		return true
	default:
		return false
	}
}

func (state *protoClientState) isDead() bool {
	return state == nil || state.dead.Load()
}

func (state *protoClientState) close() {
	if state == nil {
		return
	}
	state.closeOnce.Do(func() {
		state.dead.Store(true)
		close(state.done)
		if state.conn != nil {
			_ = state.conn.Close()
		}
	})
}

type protoHubClient struct {
	id    uint64
	state *protoClientState
}

type protoClientSnapshot struct {
	clients []protoHubClient
}

type protoClientHub struct {
	mu             sync.Mutex
	nextClientID   uint64
	clients        map[uint64]*protoClientState
	clientSnapshot atomic.Pointer[protoClientSnapshot]
	writeErrors    atomic.Uint64
	closed         bool
}

func newProtoClientHub() *protoClientHub {
	hub := &protoClientHub{clients: make(map[uint64]*protoClientState)}
	hub.clientSnapshot.Store(&protoClientSnapshot{})
	return hub
}

// publishClientSnapshotLocked replaces the immutable read-side client list.
// It must be called while hub.mu is held. Broadcast only loads this snapshot and
// never allocates or takes the hub mutex in the steady state.
func (hub *protoClientHub) publishClientSnapshotLocked() {
	clients := make([]protoHubClient, 0, len(hub.clients))
	for id, state := range hub.clients {
		clients = append(clients, protoHubClient{id: id, state: state})
	}
	hub.clientSnapshot.Store(&protoClientSnapshot{clients: clients})
}

func (hub *protoClientHub) addClient(conn protoBroadcastClient) (uint64, *protoClientState) {
	state := newProtoClientState(conn)
	if hub == nil {
		state.close()
		return 0, state
	}
	hub.mu.Lock()
	if hub.closed {
		hub.mu.Unlock()
		state.close()
		return 0, state
	}
	if hub.clients == nil {
		hub.clients = make(map[uint64]*protoClientState)
	}
	hub.nextClientID++
	id := hub.nextClientID
	hub.clients[id] = state
	hub.publishClientSnapshotLocked()
	hub.mu.Unlock()
	go hub.runClient(id, state)
	return id, state
}

func (hub *protoClientHub) runClient(id uint64, state *protoClientState) {
	pingTicker := time.NewTicker(protoClientPingPeriod)
	defer pingTicker.Stop()

	writeMessage := func(messageType int, data []byte) bool {
		if err := state.conn.SetWriteDeadline(time.Now().Add(protoClientWriteTimeout)); err != nil {
			hub.writeErrors.Add(1)
			hub.removeClient(id, state)
			return false
		}
		if err := state.conn.WriteMessage(messageType, data); err != nil {
			hub.writeErrors.Add(1)
			hub.removeClient(id, state)
			return false
		}
		return true
	}

	for {
		select {
		case <-state.done:
			return
		case <-pingTicker.C:
			if !writeMessage(websocket.PingMessage, nil) {
				return
			}
		case data := <-state.queue:
			if state.isDead() {
				return
			}
			if !writeMessage(websocket.BinaryMessage, data) {
				return
			}
		}
	}
}

func (hub *protoClientHub) removeClient(id uint64, state *protoClientState) {
	if hub == nil || state == nil {
		return
	}
	removed := false
	hub.mu.Lock()
	if current, ok := hub.clients[id]; ok && current == state {
		delete(hub.clients, id)
		hub.publishClientSnapshotLocked()
		removed = true
	}
	hub.mu.Unlock()
	if removed || !state.isDead() {
		state.close()
	}
}

// Broadcast enqueues one immutable protobuf payload per client. Slow clients
// are disconnected instead of blocking event ingestion. The read-side client
// list is immutable, so the steady-state path does not allocate or take hub.mu.
// The return value includes queue rejections and asynchronous write failures
// observed since the previous broadcast.
func (hub *protoClientHub) Broadcast(data []byte) int {
	if hub == nil || len(data) == 0 {
		return 0
	}
	writeErrors := int(hub.writeErrors.Swap(0))
	snapshot := hub.clientSnapshot.Load()
	if snapshot == nil {
		return writeErrors
	}
	for _, client := range snapshot.clients {
		if !client.state.enqueue(data) {
			writeErrors++
			hub.removeClient(client.id, client.state)
		}
	}
	return writeErrors
}

func (hub *protoClientHub) ClientCount() int {
	if hub == nil {
		return 0
	}
	snapshot := hub.clientSnapshot.Load()
	if snapshot == nil {
		return 0
	}
	return len(snapshot.clients)
}

func (hub *protoClientHub) Close() {
	if hub == nil {
		return
	}
	hub.mu.Lock()
	if hub.closed {
		hub.mu.Unlock()
		return
	}
	hub.closed = true
	snapshot := hub.clientSnapshot.Load()
	hub.clients = make(map[uint64]*protoClientState)
	hub.publishClientSnapshotLocked()
	hub.mu.Unlock()

	if snapshot == nil {
		return
	}
	for _, client := range snapshot.clients {
		client.state.close()
	}
}
