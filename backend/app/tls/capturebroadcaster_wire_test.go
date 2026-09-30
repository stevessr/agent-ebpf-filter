package tls

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// TestTLSCaptureBroadcasterRealConnectionsShareOneJSONFrame drives the
// broadcaster over real gorilla connections: every subscriber must receive
// the event as a single text frame whose JSON decodes to the original event.
func TestTLSCaptureBroadcasterRealConnectionsShareOneJSONFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upgrader := websocket.Upgrader{}
	broadcaster := newTLSCaptureBroadcasterWithUpgrader(func(w http.ResponseWriter, r *http.Request, h http.Header) (tlsBroadcastConnection, error) {
		return upgrader.Upgrade(w, r, h)
	})
	router := gin.New()
	router.GET("/ws/tls-capture", broadcaster.Serve)
	server := httptest.NewServer(router)
	defer server.Close()

	const subscribers = 3
	conns := make([]*websocket.Conn, 0, subscribers)
	for i := 0; i < subscribers; i++ {
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/tls-capture", nil)
		if err != nil {
			t.Fatalf("Dial() error = %v", err)
		}
		defer conn.Close()
		conns = append(conns, conn)
	}
	waitForTLSBroadcastClientCount(t, broadcaster, subscribers)

	event := TLSPlaintextEvent{
		Type:      "tls_plaintext",
		Timestamp: time.Unix(1_700_000_000, 0).UTC(),
		PID:       4242,
		Comm:      "curl",
		Host:      "api.example.com",
		Method:    "POST",
		URL:       "/v1/messages",
		Body:      "{\"model\":\"claude\"}",
	}
	broadcaster.Broadcast(event)

	want, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for i, conn := range conns {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("subscriber %d ReadMessage() error = %v", i, err)
		}
		if messageType != websocket.TextMessage {
			t.Fatalf("subscriber %d frame type = %d, want text", i, messageType)
		}
		if strings.TrimSpace(string(payload)) != string(want) {
			t.Fatalf("subscriber %d payload = %s, want %s", i, payload, want)
		}
		var decoded TLSPlaintextEvent
		if err := json.Unmarshal(payload, &decoded); err != nil {
			t.Fatalf("subscriber %d payload does not decode: %v", i, err)
		}
		if decoded.PID != event.PID || decoded.Host != event.Host || !decoded.Timestamp.Equal(event.Timestamp) {
			t.Fatalf("subscriber %d decoded = %+v", i, decoded)
		}
	}
	if status := broadcaster.Status(); status.ActiveClients != subscribers || status.WriteFailuresTotal != 0 || status.QueueFullDropsTotal != 0 {
		t.Fatalf("status = %+v", status)
	}
}

func TestTLSCaptureBroadcasterSkipsEncodeWithoutSubscribers(t *testing.T) {
	broadcaster := NewTLSCaptureBroadcaster()
	event := TLSPlaintextEvent{Type: "tls_plaintext", Body: strings.Repeat("x", 4096)}
	if allocs := testing.AllocsPerRun(50, func() { broadcaster.Broadcast(event) }); allocs != 0 {
		t.Fatalf("Broadcast without subscribers allocated %.1f times", allocs)
	}
}

// BenchmarkTLSCaptureBroadcasterFanOut measures one Broadcast including the
// work its writer goroutines do: each iteration waits until every
// subscriber has consumed the frame, so encode + per-client delivery is all
// inside the timed region and allocation counts cover every goroutine.
func BenchmarkTLSCaptureBroadcasterFanOut(b *testing.B) {
	for _, subscribers := range []int{1, 4, 16} {
		b.Run("subs="+strconv.Itoa(subscribers), func(b *testing.B) {
			broadcaster := NewTLSCaptureBroadcaster()
			var delivered atomic.Int64
			for i := 0; i < subscribers; i++ {
				broadcaster.addClient(&countingTLSBroadcastClient{delivered: &delivered})
			}
			b.Cleanup(broadcaster.Close)
			event := TLSPlaintextEvent{
				Type: "tls_plaintext", Timestamp: time.Unix(1_700_000_000, 0).UTC(), PID: 4242, Comm: "curl",
				Host: "api.example.com", Method: "POST", URL: "/v1/messages", Body: strings.Repeat("{\"k\":\"v\"},", 120),
			}
			b.ReportAllocs()
			var expected int64
			for b.Loop() {
				broadcaster.Broadcast(event)
				expected += int64(subscribers)
				for delivered.Load() < expected {
					runtime.Gosched()
				}
			}
		})
	}
}

// countingTLSBroadcastClient mimics what gorilla does for a prepared frame:
// the bytes are already framed, so delivery is a plain write.
type countingTLSBroadcastClient struct{ delivered *atomic.Int64 }

func (c *countingTLSBroadcastClient) WriteMessage(_ int, data []byte) error {
	_, _ = io.Discard.Write(data)
	c.delivered.Add(1)
	return nil
}
func (*countingTLSBroadcastClient) SetWriteDeadline(time.Time) error { return nil }
func (*countingTLSBroadcastClient) Close() error                     { return nil }
