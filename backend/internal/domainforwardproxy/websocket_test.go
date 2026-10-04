package domainforwardproxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestResponsesWebSocketModelRewriteRoundTrip(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upstream upgrade: %v", err)
			return
		}
		defer conn.Close()

		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("upstream read: %v", err)
			return
		}
		text := string(payload)
		if !strings.Contains(text, `"model":"fast-model"`) {
			t.Errorf("upstream did not receive redirected model: %s", text)
			return
		}
		response := []byte(`{"type":"response.completed","stream_id":"lane-1","response":{"id":"resp_1","model":"fast-model","output":[]}}`)
		if err := conn.WriteMessage(messageType, response); err != nil {
			t.Errorf("upstream write: %v", err)
		}
	}))
	defer upstream.Close()

	proxy := httptest.NewUnstartedServer(nil)
	proxyHost := NormalizeForwardHost(proxy.Listener.Addr().String())
	handler := NewHandler(DomainForwardProxySettings{
		DefaultScheme: "http",
		Routes: []DomainForwardRoute{{
			Host:     proxyHost,
			Upstream: upstream.URL,
		}},
		Rewrite: BodyRewriteSettings{
			Enabled: true,
			ModelRules: []ModelRewriteRule{{
				Host: proxyHost,
				From: "client-model",
				To:   "fast-model",
			}},
		},
	})
	proxy.Config.Handler = handler
	proxy.Start()
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	wsURL := "ws://" + proxyURL.Host + "/v1/responses"
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial proxy websocket: %v", err)
	}
	defer client.Close()

	request := []byte(`{"type":"response.create","stream_id":"lane-1","model":"client-model","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)
	if err := client.WriteMessage(websocket.TextMessage, request); err != nil {
		t.Fatalf("client write: %v", err)
	}
	_, payload, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, `"model":"client-model"`) {
		t.Fatalf("client-visible model was not restored: %s", text)
	}
	if !strings.Contains(text, `"stream_id":"lane-1"`) {
		t.Fatalf("stream_id was not preserved: %s", text)
	}
}

func TestResponsesWebSocketTargetURL(t *testing.T) {
	target, _ := url.Parse("https://provider.example/base?tenant=one")
	incoming, _ := url.Parse("https://client.example/v1/responses?trace=two")
	got := websocketTargetURL(target, incoming)
	if got.Scheme != "wss" || got.Host != "provider.example" ||
		got.Path != "/base/v1/responses" ||
		got.RawQuery != "tenant=one&trace=two" {
		t.Fatalf("unexpected websocket target: %s", got.String())
	}
}
