package main

import (
	"net/http"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/gorilla/websocket"
)

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
