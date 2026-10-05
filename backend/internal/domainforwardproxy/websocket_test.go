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

func TestResponsesWebSocketSteeringPreservesModelMapping(t *testing.T) {
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
			t.Errorf("upstream create read: %v", err)
			return
		}
		if !strings.Contains(string(payload), `"model":"fast-model"`) {
			t.Errorf("create model was not redirected: %s", payload)
			return
		}
		created := `{"type":"response.created","stream_id":"main","response":{"id":"resp_1","model":"fast-model","previous_response_id":null,"output":[]}}`
		if err := conn.WriteMessage(messageType, []byte(created)); err != nil {
			t.Errorf("upstream created write: %v", err)
			return
		}

		messageType, payload, err = conn.ReadMessage()
		if err != nil {
			t.Errorf("upstream steer read: %v", err)
			return
		}
		if !strings.Contains(string(payload), `"type":"response.steer"`) ||
			!strings.Contains(string(payload), `"previous_response_id":"resp_1"`) {
			t.Errorf("unexpected steer payload: %s", payload)
			return
		}

		accepted := `{"type":"response.steer.accepted","stream_id":"main","sequence_number":3,"steer":{"id":"steer_1","previous_response_id":"resp_1"}}`
		if err := conn.WriteMessage(messageType, []byte(accepted)); err != nil {
			t.Errorf("upstream steer accepted write: %v", err)
			return
		}
		incomplete := `{"type":"response.incomplete","stream_id":"main","response":{"id":"resp_1","model":"fast-model","previous_response_id":null,"incomplete_details":{"reason":"steered"},"output":[]}}`
		if err := conn.WriteMessage(messageType, []byte(incomplete)); err != nil {
			t.Errorf("upstream incomplete write: %v", err)
			return
		}
		successor := `{"type":"response.created","stream_id":"main","response":{"id":"resp_2","model":"fast-model","previous_response_id":"resp_1","output":[]}}`
		if err := conn.WriteMessage(messageType, []byte(successor)); err != nil {
			t.Errorf("upstream successor write: %v", err)
			return
		}
		completed := `{"type":"response.completed","stream_id":"main","response":{"id":"resp_2","model":"fast-model","previous_response_id":"resp_1","output":[]}}`
		if err := conn.WriteMessage(messageType, []byte(completed)); err != nil {
			t.Errorf("upstream successor completed write: %v", err)
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
	client, _, err := websocket.DefaultDialer.Dial("ws://"+proxyURL.Host+"/v1/responses", nil)
	if err != nil {
		t.Fatalf("dial proxy websocket: %v", err)
	}
	defer client.Close()

	create := `{"type":"response.create","stream_id":"main","model":"client-model","input":"draft"}`
	if err := client.WriteMessage(websocket.TextMessage, []byte(create)); err != nil {
		t.Fatalf("client create write: %v", err)
	}
	_, payload, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("client created read: %v", err)
	}
	if !strings.Contains(string(payload), `"model":"client-model"`) {
		t.Fatalf("initial response model was not restored: %s", payload)
	}

	steer := `{"type":"response.steer","previous_response_id":"resp_1","input":"keep it shorter"}`
	if err := client.WriteMessage(websocket.TextMessage, []byte(steer)); err != nil {
		t.Fatalf("client steer write: %v", err)
	}

	for _, eventType := range []string{"response.steer.accepted", "response.incomplete"} {
		_, payload, err = client.ReadMessage()
		if err != nil {
			t.Fatalf("client %s read: %v", eventType, err)
		}
		if !strings.Contains(string(payload), `"type":"`+eventType+`"`) {
			t.Fatalf("unexpected %s payload: %s", eventType, payload)
		}
	}
	_, payload, err = client.ReadMessage()
	if err != nil {
		t.Fatalf("client successor created read: %v", err)
	}
	if !strings.Contains(string(payload), `"id":"resp_2"`) ||
		!strings.Contains(string(payload), `"model":"client-model"`) {
		t.Fatalf("steered successor lost client model mapping: %s", payload)
	}
	_, payload, err = client.ReadMessage()
	if err != nil {
		t.Fatalf("client successor completed read: %v", err)
	}
	if !strings.Contains(string(payload), `"model":"client-model"`) {
		t.Fatalf("steered successor completion lost client model mapping: %s", payload)
	}
}

func TestResponsesWebSocketSteeringBatchState(t *testing.T) {
	state := newResponsesWSRewriteState()
	mapping := &modelRewrite{
		Client:       "client-model",
		Upstream:     "fast-model",
		clientJSON:   []byte(`"client-model"`),
		upstreamJSON: []byte(`"fast-model"`),
	}
	state.rememberResponse("resp_parent", "main", mapping)
	state.enqueueSteer("resp_parent", mapping)
	state.enqueueSteer("resp_parent", mapping)

	if !state.hasPendingSteer("resp_parent") {
		t.Fatal("expected pending steering state")
	}
	got, ok := state.takePendingSteerBatch("resp_parent")
	if !ok || got.mapping == nil || got.mapping.Client != "client-model" || got.streamID != "main" {
		t.Fatalf("unexpected transferred steering state: %#v", got)
	}
	if state.hasPendingSteer("resp_parent") {
		t.Fatal("automatic successor left stale queued steering state")
	}
}

func TestResponsesWebSocketSteerFailureTracksAcceptedID(t *testing.T) {
	state := newResponsesWSRewriteState()
	mapping := &modelRewrite{
		Client:       "client-model",
		Upstream:     "fast-model",
		clientJSON:   []byte(`"client-model"`),
		upstreamJSON: []byte(`"fast-model"`),
	}
	state.enqueueStream("main", mapping)
	state.rememberResponse("resp_parent", "main", mapping)
	state.enqueueSteer("resp_parent", mapping)
	state.enqueueSteer("resp_parent", mapping)

	if !state.acknowledgeSteer("resp_parent", "steer_1") {
		t.Fatal("first steering acknowledgement was not tracked")
	}
	if failed, ok := state.dropPendingSteer("resp_parent", ""); !ok || failed.id != "" {
		t.Fatalf("pre-accept failure did not remove unassigned steer: %#v ok=%v", failed, ok)
	}
	if !state.hasPendingSteer("resp_parent") {
		t.Fatal("accepted steer was removed by an unrelated pre-accept failure")
	}
	state.holdTerminalResponse("resp_parent", "main")
	if _, ok := state.releaseHeldTerminalResponse("resp_parent"); ok {
		t.Fatal("terminal lane released while accepted steer is still pending")
	}
	if failed, ok := state.dropPendingSteer("resp_parent", "steer_1"); !ok || failed.id != "steer_1" {
		t.Fatalf("accepted failure did not match steer id: %#v ok=%v", failed, ok)
	}
	streamID, ok := state.releaseHeldTerminalResponse("resp_parent")
	if !ok || streamID != "main" {
		t.Fatalf("terminal lane was not released after last steer failed: stream=%q ok=%v", streamID, ok)
	}
	state.popStream(streamID)
	if got := len(state.streamMappings["main"]); got != 0 {
		t.Fatalf("released terminal lane left stale mapping: len=%d", got)
	}
}

func TestResponsesWebSocketSteerFailureBeforeTerminalKeepsLane(t *testing.T) {
	state := newResponsesWSRewriteState()
	mapping := &modelRewrite{Client: "client-model", Upstream: "fast-model"}
	state.enqueueStream("main", mapping)
	state.rememberResponse("resp_parent", "main", mapping)
	state.enqueueSteer("resp_parent", mapping)
	if !state.acknowledgeSteer("resp_parent", "steer_1") {
		t.Fatal("steering acknowledgement was not tracked")
	}
	if _, ok := state.dropPendingSteer("resp_parent", "steer_1"); !ok {
		t.Fatal("accepted steer failure was not removed")
	}
	if _, ok := state.releaseHeldTerminalResponse("resp_parent"); ok {
		t.Fatal("active parent lane was released before a terminal response event")
	}
	if got := len(state.streamMappings["main"]); got != 1 {
		t.Fatalf("active parent lane mapping changed after steer failure: len=%d", got)
	}
}

func TestResponsesWebSocketPendingContinuationRejectsWrongLane(t *testing.T) {
	state := newResponsesWSRewriteState()
	parent := &modelRewrite{Client: "client-model", Upstream: "fast-model"}
	state.enqueueStream("parent-lane", parent)
	state.rememberResponse("resp_parent", "parent-lane", parent)
	state.enqueueSteer("resp_parent", parent)

	if state.bindPendingSteerContinuation("resp_parent", "wrong-lane", nil) {
		t.Fatal("wrong-lane continuation unexpectedly rebound pending steering state")
	}
	pending := state.pendingSteers["resp_parent"]
	if len(pending) != 1 {
		t.Fatalf("pending steer count = %d, want 1", len(pending))
	}
	if pending[0].streamID != "parent-lane" {
		t.Fatalf("pending steer lane changed to %q", pending[0].streamID)
	}
	if pending[0].mapping == nil || pending[0].mapping.Client != "client-model" {
		t.Fatalf("pending steer mapping changed: %#v", pending[0].mapping)
	}
}

func TestResponsesWebSocketPendingContinuationReusesQueuedLane(t *testing.T) {
	state := newResponsesWSRewriteState()
	parent := &modelRewrite{
		Client:       "client-model",
		Upstream:     "fast-model",
		clientJSON:   []byte(`"client-model"`),
		upstreamJSON: []byte(`"fast-model"`),
	}
	override := &modelRewrite{
		Client:       "other-client",
		Upstream:     "other-fast",
		clientJSON:   []byte(`"other-client"`),
		upstreamJSON: []byte(`"other-fast"`),
	}
	state.enqueueStream("main", parent)
	state.rememberResponse("resp_parent", "main", parent)
	state.enqueueSteer("resp_parent", parent)

	if !state.bindPendingSteerContinuation("resp_parent", "main", override) {
		t.Fatal("expected explicit continuation to bind queued steering state")
	}
	if got := len(state.streamMappings["main"]); got != 1 {
		t.Fatalf("explicit pending continuation duplicated lane queue: len=%d", got)
	}
	pending, ok := state.takePendingSteerBatch("resp_parent")
	if !ok || pending.mapping == nil || pending.mapping.Client != "other-client" {
		t.Fatalf("pending continuation did not adopt explicit model mapping: %#v", pending)
	}
	state.popStream("main")
	if got := len(state.streamMappings["main"]); got != 0 {
		t.Fatalf("successor completion left stale lane mapping: len=%d", got)
	}
}

func TestResponsesWebSocketExplicitContinuationModelDoesNotInheritAlias(t *testing.T) {
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
			t.Errorf("initial create read: %v", err)
			return
		}
		if !strings.Contains(string(payload), `"model":"fast-model"`) {
			t.Errorf("initial alias missing: %s", payload)
			return
		}
		if err := conn.WriteMessage(messageType, []byte(
			`{"type":"response.completed","stream_id":"main","response":{"id":"resp_1","model":"fast-model","output":[]}}`,
		)); err != nil {
			t.Errorf("initial response write: %v", err)
			return
		}

		messageType, payload, err = conn.ReadMessage()
		if err != nil {
			t.Errorf("explicit continuation read: %v", err)
			return
		}
		if !strings.Contains(string(payload), `"model":"plain-model"`) {
			t.Errorf("explicit continuation model changed unexpectedly: %s", payload)
			return
		}
		if err := conn.WriteMessage(messageType, []byte(
			`{"type":"response.completed","stream_id":"main","response":{"id":"resp_2","previous_response_id":"resp_1","model":"plain-model","output":[]}}`,
		)); err != nil {
			t.Errorf("continuation response write: %v", err)
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
	client, _, err := websocket.DefaultDialer.Dial("ws://"+proxyURL.Host+"/v1/responses", nil)
	if err != nil {
		t.Fatalf("dial proxy websocket: %v", err)
	}
	defer client.Close()

	if err := client.WriteMessage(websocket.TextMessage, []byte(
		`{"type":"response.create","stream_id":"main","model":"client-model","input":"one"}`,
	)); err != nil {
		t.Fatalf("initial create write: %v", err)
	}
	if _, _, err := client.ReadMessage(); err != nil {
		t.Fatalf("initial response read: %v", err)
	}

	if err := client.WriteMessage(websocket.TextMessage, []byte(
		`{"type":"response.create","stream_id":"main","previous_response_id":"resp_1","model":"plain-model","input":"two"}`,
	)); err != nil {
		t.Fatalf("continuation write: %v", err)
	}
	_, payload, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("continuation response read: %v", err)
	}
	if !strings.Contains(string(payload), `"model":"plain-model"`) {
		t.Fatalf("explicit continuation incorrectly inherited parent alias: %s", payload)
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
