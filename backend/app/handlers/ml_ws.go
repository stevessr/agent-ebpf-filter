package handlers

import (
	"time"

	"agent-ebpf-filter/app/wsstream"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ---- moved from app/ws_ml.go ----

// ServeMLStatusWS streams ML status updates via WebSocket using a ticker.
func ServeMLStatusWS(c *gin.Context) {
	conn, err := Deps.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(wsstream.ControlReadLimit)

	iv := wsstream.IntervalMilliseconds(c.Query("interval"), time.Second, wsstream.MinStreamInterval, wsstream.MaxStreamInterval)
	ticker := time.NewTicker(iv)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Send initial state immediately
	if err := wsstream.WriteMessage(conn, websocket.TextMessage, Deps.BuildMLStatusJSON()); err != nil {
		return
	}

	for {
		select {
		case <-ticker.C:
			if err := wsstream.WriteMessage(conn, websocket.TextMessage, Deps.BuildMLStatusJSON()); err != nil {
				return
			}
		case <-done:
			return
		case <-c.Request.Context().Done():
			return
		}
	}
}
