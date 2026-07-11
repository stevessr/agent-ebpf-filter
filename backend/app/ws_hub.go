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
	mu        sync.Mutex
	queue     chan []byte
	done      chan struct{}
	closeOnce sync.Once
	dead      bool
}

func newProtoClientState(conn protoBroadcastClient) *protoClientState {
	return &protoClientState{
		conn:  conn,
		queue: make(chan []byte, protoClientQueueSize),
		done:  make(chan struct{}),
	}
}

func (state *protoClientState) enqueue(data []byte) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.dead {
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
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.dead
}

func (state *protoClientState) close() {
	state.closeOnce.Do(func() {
		state.mu.Lock()
		state.dead = true
		close(state.done)
		state.mu.Unlock()
		_ = state.conn.Close()
	})
}

type protoClientHub struct {
	mu           sync.Mutex
	nextClientID uint64
	clients      map[uint64]*protoClientState
	writeErrors  atomic.Uint64
	closed       bool
}

func newProtoClientHub() *protoClientHub {
	return &protoClientHub{clients: make(map[uint64]*protoClientState)}
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
	hub.mu.Lock()
	if current, ok := hub.clients[id]; ok && current == state {
		delete(hub.clients, id)
	}
	hub.mu.Unlock()
	state.close()
}

// Broadcast enqueues one immutable protobuf payload per client. Slow clients
// are disconnected instead of blocking event ingestion. The return value
// includes queue rejections and asynchronous write failures observed since the
// previous broadcast.
func (hub *protoClientHub) Broadcast(data []byte) int {
	if hub == nil || len(data) == 0 {
		return 0
	}
	type client struct {
		id    uint64
		state *protoClientState
	}

	writeErrors := int(hub.writeErrors.Swap(0))
	hub.mu.Lock()
	if hub.closed {
		hub.mu.Unlock()
		return writeErrors
	}
	clients := make([]client, 0, len(hub.clients))
	for id, state := range hub.clients {
		clients = append(clients, client{id: id, state: state})
	}
	hub.mu.Unlock()

	for _, client := range clients {
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
	hub.mu.Lock()
	defer hub.mu.Unlock()
	return len(hub.clients)
}

func (hub *protoClientHub) Close() {
	if hub == nil {
		return
	}
	type client struct {
		id    uint64
		state *protoClientState
	}
	hub.mu.Lock()
	if hub.closed {
		hub.mu.Unlock()
		return
	}
	hub.closed = true
	clients := make([]client, 0, len(hub.clients))
	for id, state := range hub.clients {
		clients = append(clients, client{id: id, state: state})
	}
	hub.mu.Unlock()
	for _, client := range clients {
		hub.removeClient(client.id, client.state)
	}
}
