package core

import (
	"net/http"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/gorilla/websocket"
)

// ── WebSocket state ──────────────────────────────────────────────────────────

var (
	Clients           = make(map[*websocket.Conn]bool)
	ClientsMu         sync.Mutex
	EnvelopeClients   = make(map[*websocket.Conn]bool)
	EnvelopeClientsMu sync.Mutex
	Broadcast         = make(chan *pb.Event, 1000)

	Upgrader = websocket.Upgrader{
		CheckOrigin:      func(r *http.Request) bool { return true },
		ReadBufferSize:   1024 * 32,
		WriteBufferSize:  1024 * 1024,
		HandshakeTimeout: 5 * time.Second,
	}
)
