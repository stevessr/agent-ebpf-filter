package main

import (
	"sync"
	"github.com/gorilla/websocket"
	"agent-ebpf-filter/pb"
	"net/http"
)

var (
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex
	broadcast = make(chan *pb.Event, 1000)
	
	upgrader = websocket.Upgrader{
		CheckOrigin:     func(r *http.Request) bool { return true },
		ReadBufferSize:  1024 * 16,
		WriteBufferSize: 1024 * 1024, // 1MB buffer for video frames
	}
)
