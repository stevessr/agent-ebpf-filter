// Package wsfanout implements a bounded, non-blocking WebSocket fan-out hub.
//
// A Hub owns one writer goroutine per client and never blocks the producer:
// a client whose queue is full, or whose write fails, is disconnected so event
// ingestion keeps its latency budget. Every Broadcast shares a single Message
// between all clients; the websocket frame is encoded once and written with a
// vectored write per client instead of being re-framed and copied per client.
package wsfanout

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	DefaultQueueSize    = 16
	DefaultWriteTimeout = 2 * time.Second
	DefaultPingPeriod   = 30 * time.Second
)

// Conn is the minimal connection surface the hub writes to. *websocket.Conn
// satisfies it; tests use in-memory fakes.
type Conn interface {
	WriteMessage(messageType int, data []byte) error
	SetWriteDeadline(deadline time.Time) error
	Close() error
}

// preparedConn is satisfied by *websocket.Conn. When available the hub writes
// the shared pre-framed payload directly instead of re-encoding it per client.
type preparedConn interface {
	WritePreparedMessage(*websocket.PreparedMessage) error
}

// Message is an immutable payload shared by every client of a broadcast. The
// caller must not modify data after handing it to the hub.
type Message struct {
	messageType int
	data        []byte
	prepareOnce sync.Once
	prepared    *websocket.PreparedMessage
	prepareErr  error
}

// NewBinaryMessage wraps data as a websocket binary message.
func NewBinaryMessage(data []byte) *Message {
	return &Message{messageType: websocket.BinaryMessage, data: data}
}

// NewTextMessage wraps data as a websocket text message.
func NewTextMessage(data []byte) *Message {
	return &Message{messageType: websocket.TextMessage, data: data}
}

// Bytes returns the raw payload. Callers must treat it as read-only.
func (m *Message) Bytes() []byte { return m.data }

func (m *Message) preparedMessage() (*websocket.PreparedMessage, error) {
	m.prepareOnce.Do(func() {
		m.prepared, m.prepareErr = websocket.NewPreparedMessage(m.messageType, m.data)
	})
	return m.prepared, m.prepareErr
}

func (m *Message) writeTo(conn Conn) error {
	if pc, ok := conn.(preparedConn); ok {
		if prepared, err := m.preparedMessage(); err == nil {
			return pc.WritePreparedMessage(prepared)
		}
	}
	return conn.WriteMessage(m.messageType, m.data)
}

// DropReason says why a client was disconnected by the hub.
type DropReason uint8

const (
	// DropQueueFull: the client's queue was full when a message arrived.
	DropQueueFull DropReason = iota + 1
	// DropDeadlineFailure: SetWriteDeadline failed before a write.
	DropDeadlineFailure
	// DropWriteFailure: the write itself failed.
	DropWriteFailure
)

func (r DropReason) String() string {
	switch r {
	case DropQueueFull:
		return "queue_full"
	case DropDeadlineFailure:
		return "write_deadline_failure"
	case DropWriteFailure:
		return "write_failure"
	default:
		return "unknown"
	}
}

// Options tunes a Hub. Zero values fall back to the package defaults; a
// negative PingPeriod disables keep-alive pings.
type Options struct {
	QueueSize    int
	WriteTimeout time.Duration
	PingPeriod   time.Duration
	// OnDrop, when set, is called once for every client the hub disconnects
	// on its own initiative. Failures observed on a client that was already
	// removed (for example the write that fails because we closed it) are
	// not reported.
	OnDrop func(DropReason)
}

func (o Options) withDefaults() Options {
	if o.QueueSize <= 0 {
		o.QueueSize = DefaultQueueSize
	}
	if o.WriteTimeout <= 0 {
		o.WriteTimeout = DefaultWriteTimeout
	}
	if o.PingPeriod == 0 {
		o.PingPeriod = DefaultPingPeriod
	}
	return o
}

// Client is one registered connection. It is returned by Hub.Add and handed
// back to Hub.Remove when the connection's read loop ends.
type Client struct {
	id        uint64
	conn      Conn
	mu        sync.Mutex
	queue     chan *Message
	done      chan struct{}
	closeOnce sync.Once
	dead      bool
}

func newClient(conn Conn, queueSize int) *Client {
	return &Client{
		conn:  conn,
		queue: make(chan *Message, queueSize),
		done:  make(chan struct{}),
	}
}

// Alive reports whether the client is still registered and writable.
func (c *Client) Alive() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.dead
}

func (c *Client) enqueue(msg *Message) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead {
		return false
	}
	select {
	case c.queue <- msg:
		return true
	default:
		return false
	}
}

func (c *Client) close() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.dead = true
		close(c.done)
		c.mu.Unlock()
		_ = c.conn.Close()
	})
}

// Hub fans messages out to a dynamic set of clients.
type Hub struct {
	opts        Options
	mu          sync.Mutex
	nextID      uint64
	clients     map[uint64]*Client
	writeErrors atomic.Uint64
	closed      bool

	// broadcastMu serialises Broadcast calls so the client scratch slice can
	// be reused instead of allocated per call. Enqueueing is non-blocking,
	// so the critical section is short.
	broadcastMu sync.Mutex
	scratch     []*Client
}

// New returns an open hub.
func New(opts Options) *Hub {
	return &Hub{opts: opts.withDefaults(), clients: make(map[uint64]*Client)}
}

// Add registers conn and starts its writer goroutine. If the hub is already
// closed the connection is closed immediately and the returned client is dead.
func (h *Hub) Add(conn Conn) *Client {
	if h == nil {
		client := newClient(conn, 0)
		client.close()
		return client
	}
	client := newClient(conn, h.opts.QueueSize)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		client.close()
		return client
	}
	h.nextID++
	client.id = h.nextID
	h.clients[client.id] = client
	h.mu.Unlock()
	go h.runClient(client)
	return client
}

func (h *Hub) runClient(client *Client) {
	var ping <-chan time.Time
	if h.opts.PingPeriod > 0 {
		ticker := time.NewTicker(h.opts.PingPeriod)
		defer ticker.Stop()
		ping = ticker.C
	}

	for {
		select {
		case <-client.done:
			return
		case <-ping:
			if !h.write(client, nil) {
				return
			}
		case msg := <-client.queue:
			if !client.Alive() {
				return
			}
			if !h.write(client, msg) {
				return
			}
		}
	}
}

// write delivers msg to client, or a ping when msg is nil, and removes the
// client on failure. It is a method rather than a closure so the hot path
// does not allocate per message.
func (h *Hub) write(client *Client, msg *Message) bool {
	if err := client.conn.SetWriteDeadline(time.Now().Add(h.opts.WriteTimeout)); err != nil {
		h.fail(client, DropDeadlineFailure)
		return false
	}
	var err error
	if msg == nil {
		err = client.conn.WriteMessage(websocket.PingMessage, nil)
	} else {
		err = msg.writeTo(client.conn)
	}
	if err != nil {
		h.fail(client, DropWriteFailure)
		return false
	}
	return true
}

func (h *Hub) fail(client *Client, reason DropReason) {
	if client.Alive() {
		h.writeErrors.Add(1)
		h.notifyDrop(reason)
	}
	h.Remove(client)
}

// Remove unregisters client and closes its connection. It is safe to call
// more than once and for clients that were never registered.
func (h *Hub) Remove(client *Client) {
	if h == nil || client == nil {
		return
	}
	h.mu.Lock()
	if current, ok := h.clients[client.id]; ok && current == client {
		delete(h.clients, client.id)
	}
	h.mu.Unlock()
	client.close()
}

// snapshotInto appends the live clients to dst[:0]. It returns nil once the
// hub is closed.
func (h *Hub) snapshotInto(dst []*Client) []*Client {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	dst = dst[:0]
	for _, client := range h.clients {
		dst = append(dst, client)
	}
	return dst
}

// Broadcast enqueues msg for every client. Slow clients are disconnected
// instead of blocking the caller. The return value counts queue rejections
// plus asynchronous write failures observed since the previous broadcast.
func (h *Hub) Broadcast(msg *Message) int {
	if h == nil || msg == nil || len(msg.data) == 0 {
		return 0
	}
	h.broadcastMu.Lock()
	defer h.broadcastMu.Unlock()
	failures := int(h.writeErrors.Swap(0))
	h.scratch = h.snapshotInto(h.scratch)
	for _, client := range h.scratch {
		if !client.enqueue(msg) {
			failures++
			if client.Alive() {
				h.notifyDrop(DropQueueFull)
			}
			h.Remove(client)
		}
	}
	// Do not keep removed clients reachable from the scratch slice.
	clear(h.scratch)
	return failures
}

func (h *Hub) notifyDrop(reason DropReason) {
	if h.opts.OnDrop != nil {
		h.opts.OnDrop(reason)
	}
}

// Queued returns the number of messages waiting in client queues.
func (h *Hub) Queued() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	queued := 0
	for _, client := range h.clients {
		queued += len(client.queue)
	}
	return queued
}

// DisconnectAll removes every client but keeps the hub open for new ones.
func (h *Hub) DisconnectAll() {
	if h == nil {
		return
	}
	h.mu.Lock()
	clients := make([]*Client, 0, len(h.clients))
	for _, client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.Unlock()
	for _, client := range clients {
		h.Remove(client)
	}
}

// Len returns the number of registered clients.
func (h *Hub) Len() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// Closed reports whether Close has been called.
func (h *Hub) Closed() bool {
	if h == nil {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

// Close disconnects every client and rejects further Add calls.
func (h *Hub) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	clients := make([]*Client, 0, len(h.clients))
	for _, client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.Unlock()
	for _, client := range clients {
		h.Remove(client)
	}
}
