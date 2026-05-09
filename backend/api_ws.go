package main

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func serveEventsWS(c *gin.Context) {
	servePassiveProtoWS(c, clients, &clientsMu)
}

func serveEventEnvelopesWS(c *gin.Context) {
	servePassiveProtoWS(c, envelopeClients, &envelopeClientsMu)
}

func servePassiveProtoWS(c *gin.Context, target map[*websocket.Conn]bool, mu *sync.Mutex) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	mu.Lock()
	target[conn] = true
	mu.Unlock()

	go func(conn *websocket.Conn) {
		defer func() {
			mu.Lock()
			delete(target, conn)
			mu.Unlock()
			_ = conn.Close()
		}()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}(conn)
}

func broadcastProtoMessage(target map[*websocket.Conn]bool, mu *sync.Mutex, data []byte) {
	mu.Lock()
	for conn := range target {
		if conn == nil {
			delete(target, conn)
			continue
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			conn.Close()
			delete(target, conn)
		}
	}
	mu.Unlock()
}

func startEventBroadcaster() {
	go func() {
		eventBatch := make([]*pb.Event, 0, 50)
		envelopeBatch := make([]*pb.EventEnvelope, 0, 50)
		batchTicker := time.NewTicker(50 * time.Millisecond)
		defer batchTicker.Stop()

		flushBatch := func() {
			if len(eventBatch) > 0 {
				events := make([]*pb.Event, len(eventBatch))
				copy(events, eventBatch)
				eventBatch = eventBatch[:0]
				msg := &pb.EventBatch{Events: events}
				data, err := proto.Marshal(msg)
				if err != nil {
					log.Printf("[ERROR] failed to marshal EventBatch: %v", err)
				} else {
					broadcastProtoMessage(clients, &clientsMu, data)
				}
			}
			if len(envelopeBatch) > 0 {
				envelopes := make([]*pb.EventEnvelope, len(envelopeBatch))
				copy(envelopes, envelopeBatch)
				envelopeBatch = envelopeBatch[:0]
				msg := &pb.EventEnvelopeBatch{Envelopes: envelopes}
				data, err := proto.Marshal(msg)
				if err != nil {
					log.Printf("[ERROR] failed to marshal EventEnvelopeBatch: %v", err)
				} else {
					broadcastProtoMessage(envelopeClients, &envelopeClientsMu, data)
				}
			}
		}

		appendRecord := func(record CapturedEventRecord) {
			if record.Event != nil {
				eventBatch = append(eventBatch, record.Event)
			}
			if record.Envelope != nil {
				envelopeBatch = append(envelopeBatch, record.Envelope)
			}
		}

		for {
			select {
			case event := <-broadcast:
				event = enrichEventContext(event)
				appendRecord(recordCapturedEvent(event))
				for _, alert := range buildSemanticAlerts(event) {
					alert = enrichEventContext(alert)
					appendRecord(recordCapturedEvent(alert))
				}
				if len(eventBatch) >= 50 || len(envelopeBatch) >= 50 {
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
		for _, record := range records {
			record = normalizeCapturedEventRecord(record)
			if record.Event != nil && record.Event.Type == typeFilter {
				filtered = append(filtered, record)
			}
		}
		records = filtered
	}

	resp := &pb.EventHistoryResponse{Source: source}
	for _, record := range records {
		record = normalizeCapturedEventRecord(record)
		resp.Events = append(resp.Events, &pb.CapturedEventRecord{
			Event:     record.Event,
			Timestamp: record.ReceivedAt.UnixMilli(),
			Envelope:  record.Envelope,
		})
	}
	writeProtoOrJSON(c, http.StatusOK, resp, gin.H{"source": source, "events": buildCapturedEventJSONRecords(records)})
}
