package handlers

import (
	"time"

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

	intervalStr := c.DefaultQuery("interval", "1000")
	iv, _ := time.ParseDuration(intervalStr + "ms")
	if iv < 500*time.Millisecond {
		iv = 500 * time.Millisecond
	}
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
	if err := conn.WriteMessage(websocket.TextMessage, Deps.BuildMLStatusJSON()); err != nil {
		return
	}

	for {
		select {
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.TextMessage, Deps.BuildMLStatusJSON()); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
