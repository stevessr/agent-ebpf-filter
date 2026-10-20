package app

import (
	"agent-ebpf-filter/pb"
	"time"

	"github.com/gorilla/websocket"
)

// Package-level event runtime initialized once and bound into AppContext.
var (
	broadcast = make(chan *pb.Event, 1000)

	upgrader = websocket.Upgrader{
		CheckOrigin:      allowWebSocketOrigin,
		ReadBufferSize:   1024 * 32,
		WriteBufferSize:  1024 * 1024,
		HandshakeTimeout: 5 * time.Second,
	}
)
