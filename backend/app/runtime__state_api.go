package app

import (
	"agent-ebpf-filter/pb"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ---- moved from backend/zz_merged_backend.go section state_api.go ----

var (
	clients           = make(map[*websocket.Conn]bool)
	clientsMu         sync.Mutex
	envelopeClients   = make(map[*websocket.Conn]bool)
	envelopeClientsMu sync.Mutex
	broadcast         = make(chan *pb.Event, 1000)

	upgrader = websocket.Upgrader{
		CheckOrigin:      func(r *http.Request) bool { return true },
		ReadBufferSize:   1024 * 32,
		WriteBufferSize:  1024 * 1024,
		HandshakeTimeout: 5 * time.Second,
	}
)
