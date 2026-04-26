package main

import (
	"net/http"
	"strconv"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func serveEventsWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()

	go func(conn *websocket.Conn) {
		defer func() {
			clientsMu.Lock()
			delete(clients, conn)
			clientsMu.Unlock()
			_ = conn.Close()
		}()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}(conn)
}

func startEventBroadcaster() {
	go func() {
		batch := make([]*pb.Event, 0, 50)
		batchTicker := time.NewTicker(50 * time.Millisecond)
		defer batchTicker.Stop()
		flushBatch := func() {
			if len(batch) == 0 {
				return
			}
			events := make([]*pb.Event, len(batch))
			copy(events, batch)
			batch = batch[:0]
			msg := &pb.EventBatch{Events: events}
			data, _ := proto.Marshal(msg)
			clientsMu.Lock()
			for c := range clients {
				if c == nil {
					delete(clients, c)
					continue
				}
				if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
					c.Close()
					delete(clients, c)
				}
			}
			clientsMu.Unlock()
		}
		for {
			select {
			case event := <-broadcast:
				recordCapturedEvent(event)
				batch = append(batch, event)
				if len(batch) >= 50 {
					flushBatch()
				}
			case <-batchTicker.C:
				flushBatch()
			}
		}
	}()
}

func handleRecentEvents(c *gin.Context) {
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	typeFilter := c.Query("type")
	records, source, err := runtimeSettingsStore.RecentEvents(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if typeFilter != "" {
		filtered := make([]CapturedEventRecord, 0, len(records))
		for _, r := range records {
			if r.Event != nil && r.Event.Type == typeFilter {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}
	c.JSON(http.StatusOK, gin.H{"source": source, "events": records})
}
