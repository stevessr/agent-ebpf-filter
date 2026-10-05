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
			t.Errorf("upstream first read: %v", err)
			return
		}
		text := string(payload)
		if !strings.Contains(text, `"model":"fast-model"`) {
			t.Errorf("upstream did not receive redirected model: %s", text)
			return
		}
		response := []byte(`{"type":"response.completed","stream_id":"lane-1","response":{"id":"resp_1","model":"fast-model","output":[]}}`)
		if err := conn.WriteMessage(messageType, response); err != nil {
			t.Errorf("upstream first write: %v", err)
			return
		}

		messageType, payload, err = conn.ReadMessage()
		if err != nil {
			t.Errorf("upstream continuation read: %v", err)
			return
		}
		text = string(payload)
		if strings.Contains(text, `"model"`) ||
			!strings.Contains(text, `"previous_response_id":"resp_1"`) {
			t.Errorf("unexpected continuation request: %s", text)
			return
		}
		response = []byte(`{"type":"response.completed","stream_id":"lane-1","response":{"id":"resp_2","model":"fast-model","output":[]}}`)
		if err := conn.WriteMessage(messageType, response); err != nil {
			t.Errorf("upstream continuation write: %v", err)
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

	continuation := []byte(`{"type":"response.create","stream_id":"lane-1","previous_response_id":"resp_1","input":[{"role":"user","content":[{"type":"input_text","text":"continue"}]}]}`)
	if err := client.WriteMessage(websocket.TextMessage, continuation); err != nil {
		t.Fatalf("client continuation write: %v", err)
	}
	_, payload, err = client.ReadMessage()
	if err != nil {
		t.Fatalf("client continuation read: %v", err)
	}
	text = string(payload)
	if !strings.Contains(text, `"model":"client-model"`) ||
		!strings.Contains(text, `"id":"resp_2"`) {
		t.Fatalf("continuation did not inherit model mapping: %s", text)
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

func TestResponsesWebSocketQueuesModelMappingsPerLane(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upstream upgrade: %v", err)
			return
		}
		defer conn.Close()

		for _, want := range []string{`"model":"fast-a"`, `"model":"fast-b"`} {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				t.Errorf("upstream read: %v", err)
				return
			}
			if !strings.Contains(string(payload), want) {
				t.Errorf("upstream payload %s does not contain %s", payload, want)
				return
			}
		}

		for _, response := range []string{
			`{"type":"response.completed","stream_id":"main","response":{"id":"resp_a","model":"fast-a","output":[]}}`,
			`{"type":"response.completed","stream_id":"main","response":{"id":"resp_b","model":"fast-b","output":[]}}`,
		} {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(response)); err != nil {
				t.Errorf("upstream write: %v", err)
				return
			}
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
			ModelRules: []ModelRewriteRule{
				{Host: proxyHost, From: "client-a", To: "fast-a"},
				{Host: proxyHost, From: "client-b", To: "fast-b"},
			},
		},
	})
	proxy.Config.Handler = handler
	proxy.Start()
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	client, _, err := websocket.DefaultDialer.Dial("ws://"+proxyURL.Host+"/v1/responses", nil)
	if err != nil {
		t.Fatalf("dial proxy websocket: %v", err)
	}
	defer client.Close()

	for _, request := range []string{
		`{"type":"response.create","stream_id":"main","model":"client-a","input":"one"}`,
		`{"type":"response.create","stream_id":"main","model":"client-b","input":"two"}`,
	} {
		if err := client.WriteMessage(websocket.TextMessage, []byte(request)); err != nil {
			t.Fatalf("client write: %v", err)
		}
	}

	for _, want := range []string{`"model":"client-a"`, `"model":"client-b"`} {
		_, payload, err := client.ReadMessage()
		if err != nil {
			t.Fatalf("client read: %v", err)
		}
		if !strings.Contains(string(payload), want) {
			t.Fatalf("client payload %s does not contain %s", payload, want)
		}
	}
}

func TestResponsesWebSocketTerminalEvents(t *testing.T) {
	cases := []struct {
		name       string
		eventType  string
		streamID   string
		errorParam string
		want       bool
	}{
		{name: "completed", eventType: "response.completed", streamID: "main", want: true},
		{name: "failed", eventType: "response.failed", streamID: "main", want: true},
		{name: "incomplete", eventType: "response.incomplete", streamID: "main", want: true},
		{name: "named lane error", eventType: "error", streamID: "main", want: true},
		{name: "default lane request error", eventType: "error", want: true},
		{name: "invalid stream id is unbound", eventType: "error", errorParam: "stream_id", want: false},
		{name: "delta", eventType: "response.output_text.delta", streamID: "main", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := responsesTerminalEvent(tc.eventType, tc.streamID, tc.errorParam); got != tc.want {
				t.Fatalf("responsesTerminalEvent(%q, %q, %q) = %v, want %v", tc.eventType, tc.streamID, tc.errorParam, got, tc.want)
			}
		})
	}
}
