package app

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestParseEventLimitQueryIsBounded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want int
	}{
		{raw: "", want: 50},
		{raw: "25", want: 25},
		{raw: "0", want: maxRecentEventLimit},
		{raw: "all", want: maxRecentEventLimit},
		{raw: "unlimited", want: maxRecentEventLimit},
		{raw: "2147483647", want: maxRecentEventLimit},
		{raw: "-1", want: 50},
		{raw: "invalid", want: 50},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			if got := parseEventLimitQuery(tc.raw, 50); got != tc.want {
				t.Fatalf("parseEventLimitQuery(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestPassiveProtoWSRejectsOversizedInboundFrame(t *testing.T) {
	hub := newProtoClientHub()
	t.Cleanup(hub.Close)
	router := gin.New()
	router.GET("/events", func(c *gin.Context) {
		servePassiveProtoWS(c, hub)
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/events"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, passiveProtoWSReadLimit+1)); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("oversized inbound frame did not close the passive websocket")
	}

	deadline := time.Now().Add(2 * time.Second)
	for hub.ClientCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if hub.ClientCount() != 0 {
		t.Fatal("oversized-frame client remained registered")
	}
}
