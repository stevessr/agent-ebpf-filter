package tls

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2/hpack"
)

func encodeTestHPACK(t *testing.T, encoder *hpack.Encoder, buffer *bytes.Buffer, fields ...hpack.HeaderField) []byte {
	t.Helper()
	buffer.Reset()
	for _, field := range fields {
		if err := encoder.WriteField(field); err != nil {
			t.Fatalf("WriteField(%s): %v", field.Name, err)
		}
	}
	return append([]byte(nil), buffer.Bytes()...)
}

func testHTTP2MetaFragment(direction uint8, connectionID uint64) CompletedTLSFragment {
	fragment := testCompletedTLSFragment("", direction)
	fragment.ConnectionID = connectionID
	fragment.TimestampNS = uint64(time.Now().Add(-time.Millisecond).UnixNano())
	return fragment
}

func TestTLSHTTP2HeaderDecoderRedactsRequestHeadersAndURL(t *testing.T) {
	decoder := newTLSHTTP2HeaderDecoder(10 * time.Second)
	var encoded bytes.Buffer
	encoder := hpack.NewEncoder(&encoded)
	block := encodeTestHPACK(t, encoder, &encoded,
		hpack.HeaderField{Name: ":method", Value: "POST"},
		hpack.HeaderField{Name: ":path", Value: "/v1/messages?api_key=secret&safe=yes"},
		hpack.HeaderField{Name: ":authority", Value: "api.example.com"},
		hpack.HeaderField{Name: "content-type", Value: "application/json"},
		hpack.HeaderField{Name: "authorization", Value: "Bearer top-secret"},
	)
	frame := testTLSHTTP2Frame(0x1, 0x4, 1, block)
	meta := testHTTP2MetaFragment(tlsDirectionSend, 0x501)
	key := tlsHTTP2StreamKeyFor(meta)
	event := decoder.enrichFrame(key, tlsHTTP2FrameEvent(meta, frame, 0), frame)

	if event.Type != "http2_request" || event.Method != "POST" || event.Host != "api.example.com" {
		t.Fatalf("unexpected request event: %+v", event)
	}
	if strings.Contains(event.URL, "secret") || !strings.Contains(event.URL, "safe=yes") {
		t.Fatalf("URL was not sanitized correctly: %q", event.URL)
	}
	if event.Headers["authorization"] != tlsRedactedValue {
		t.Fatalf("authorization = %q, want redacted", event.Headers["authorization"])
	}
	if event.RawHexDump != "" || event.RedactionState != "sanitized" {
		t.Fatalf("raw/redaction state = %q/%q", event.RawHexDump, event.RedactionState)
	}
	if event.HTTP2StreamID != 1 || event.HTTP2FrameType != "headers" {
		t.Fatalf("HTTP2 metadata = stream=%d type=%q", event.HTTP2StreamID, event.HTTP2FrameType)
	}
}

func TestTLSHTTP2HeaderDecoderCorrelatesResponseAndSanitizesData(t *testing.T) {
	decoder := newTLSHTTP2HeaderDecoder(10 * time.Second)
	connectionID := uint64(0x502)

	var requestBuffer bytes.Buffer
	requestEncoder := hpack.NewEncoder(&requestBuffer)
	requestBlock := encodeTestHPACK(t, requestEncoder, &requestBuffer,
		hpack.HeaderField{Name: ":method", Value: "POST"},
		hpack.HeaderField{Name: ":path", Value: "/v1/messages"},
		hpack.HeaderField{Name: ":authority", Value: "api.example.com"},
	)
	requestFrame := testTLSHTTP2Frame(0x1, 0x4, 3, requestBlock)
	requestMeta := testHTTP2MetaFragment(tlsDirectionSend, connectionID)
	requestKey := tlsHTTP2StreamKeyFor(requestMeta)
	request := decoder.enrichFrame(requestKey, tlsHTTP2FrameEvent(requestMeta, requestFrame, 0), requestFrame)
	if request.Type != "http2_request" {
		t.Fatalf("request type = %q", request.Type)
	}

	var responseBuffer bytes.Buffer
	responseEncoder := hpack.NewEncoder(&responseBuffer)
	responseBlock := encodeTestHPACK(t, responseEncoder, &responseBuffer,
		hpack.HeaderField{Name: ":status", Value: "200"},
		hpack.HeaderField{Name: "content-type", Value: "application/json"},
	)
	responseFrame := testTLSHTTP2Frame(0x1, 0x4, 3, responseBlock)
	responseMeta := testHTTP2MetaFragment(tlsDirectionRecv, connectionID)
	responseKey := tlsHTTP2StreamKeyFor(responseMeta)
	response := decoder.enrichFrame(responseKey, tlsHTTP2FrameEvent(responseMeta, responseFrame, 0), responseFrame)
	if response.Type != "http2_response" || response.StatusCode != 200 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Method != "POST" || response.URL != "/v1/messages" || response.Host != "api.example.com" {
		t.Fatalf("response lost request context: %+v", response)
	}

	dataFrame := testTLSHTTP2Frame(0x0, 0x1, 3, []byte(`{"token":"super-secret","text":"ok"}`))
	dataEvent := decoder.enrichFrame(responseKey, tlsHTTP2FrameEvent(responseMeta, dataFrame, 0), dataFrame)
	if dataEvent.Method != "POST" || dataEvent.StatusCode != 200 || dataEvent.ContentType != "application/json" {
		t.Fatalf("DATA lost stream context: %+v", dataEvent)
	}
	if strings.Contains(dataEvent.Body, "super-secret") || !strings.Contains(dataEvent.Body, tlsRedactedValue) {
		t.Fatalf("DATA body was not sanitized: %q", dataEvent.Body)
	}
	if dataEvent.RawHexDump != "" || dataEvent.RedactionState != "sanitized" {
		t.Fatalf("DATA raw/redaction state = %q/%q", dataEvent.RawHexDump, dataEvent.RedactionState)
	}
}

func TestTLSHTTP2HeaderDecoderKeepsDuplexDirectionsSeparate(t *testing.T) {
	decoder := newTLSHTTP2HeaderDecoder(10 * time.Second)
	connectionID := uint64(0x505)
	streamID := uint32(11)

	var requestBuffer bytes.Buffer
	requestEncoder := hpack.NewEncoder(&requestBuffer)
	requestBlock := encodeTestHPACK(t, requestEncoder, &requestBuffer,
		hpack.HeaderField{Name: ":method", Value: "POST"},
		hpack.HeaderField{Name: ":path", Value: "/stream"},
		hpack.HeaderField{Name: ":authority", Value: "api.example.com"},
		hpack.HeaderField{Name: "content-type", Value: "application/x-ndjson"},
	)
	requestMeta := testHTTP2MetaFragment(tlsDirectionSend, connectionID)
	requestKey := tlsHTTP2StreamKeyFor(requestMeta)
	requestFrame := testTLSHTTP2Frame(0x1, 0x4, streamID, requestBlock)
	decoder.enrichFrame(requestKey, tlsHTTP2FrameEvent(requestMeta, requestFrame, 0), requestFrame)

	var responseBuffer bytes.Buffer
	responseEncoder := hpack.NewEncoder(&responseBuffer)
	responseBlock := encodeTestHPACK(t, responseEncoder, &responseBuffer,
		hpack.HeaderField{Name: ":status", Value: "200"},
		hpack.HeaderField{Name: "content-type", Value: "application/json"},
	)
	responseMeta := testHTTP2MetaFragment(tlsDirectionRecv, connectionID)
	responseKey := tlsHTTP2StreamKeyFor(responseMeta)
	responseFrame := testTLSHTTP2Frame(0x1, 0x4, streamID, responseBlock)
	decoder.enrichFrame(responseKey, tlsHTTP2FrameEvent(responseMeta, responseFrame, 0), responseFrame)

	requestDataFrame := testTLSHTTP2Frame(0x0, 0x1, streamID, []byte(`{"input":"hello"}`))
	requestData := decoder.enrichFrame(requestKey, tlsHTTP2FrameEvent(requestMeta, requestDataFrame, 0), requestDataFrame)
	if requestData.StatusCode != 0 {
		t.Fatalf("request DATA inherited response status %d", requestData.StatusCode)
	}
	if requestData.ContentType != "application/x-ndjson" {
		t.Fatalf("request DATA content type = %q", requestData.ContentType)
	}

	responseDataFrame := testTLSHTTP2Frame(0x0, 0x1, streamID, []byte(`{"result":"ok"}`))
	responseData := decoder.enrichFrame(responseKey, tlsHTTP2FrameEvent(responseMeta, responseDataFrame, 0), responseDataFrame)
	if responseData.StatusCode != 200 || responseData.ContentType != "application/json" {
		t.Fatalf("response DATA lost context after request END_STREAM: %+v", responseData)
	}
	if responseData.Method != "POST" || responseData.URL != "/stream" {
		t.Fatalf("response DATA lost request identity: %+v", responseData)
	}
}

func TestTLSHTTP2HeaderDecoderReassemblesContinuation(t *testing.T) {
	decoder := newTLSHTTP2HeaderDecoder(10 * time.Second)
	var encoded bytes.Buffer
	encoder := hpack.NewEncoder(&encoded)
	block := encodeTestHPACK(t, encoder, &encoded,
		hpack.HeaderField{Name: ":method", Value: "GET"},
		hpack.HeaderField{Name: ":path", Value: "/models"},
		hpack.HeaderField{Name: ":authority", Value: "api.example.com"},
		hpack.HeaderField{Name: "x-long", Value: strings.Repeat("value", 20)},
	)
	cut := len(block) / 2
	if cut == 0 {
		t.Fatal("encoded header block unexpectedly empty")
	}
	meta := testHTTP2MetaFragment(tlsDirectionSend, 0x503)
	key := tlsHTTP2StreamKeyFor(meta)
	firstFrame := testTLSHTTP2Frame(0x1, 0, 5, block[:cut])
	first := decoder.enrichFrame(key, tlsHTTP2FrameEvent(meta, firstFrame, 0), firstFrame)
	if first.Type != "http2_frame" {
		t.Fatalf("first frame type = %q, want generic while waiting", first.Type)
	}
	secondFrame := testTLSHTTP2Frame(0x9, 0x4, 5, block[cut:])
	second := decoder.enrichFrame(key, tlsHTTP2FrameEvent(meta, secondFrame, 0), secondFrame)
	if second.Type != "http2_request" || second.Method != "GET" || second.URL != "/models" {
		t.Fatalf("continuation result: %+v", second)
	}
}

func TestTLSHTTP2HeaderDecoderReusesDynamicTable(t *testing.T) {
	decoder := newTLSHTTP2HeaderDecoder(10 * time.Second)
	var encoded bytes.Buffer
	encoder := hpack.NewEncoder(&encoded)
	meta := testHTTP2MetaFragment(tlsDirectionSend, 0x504)
	key := tlsHTTP2StreamKeyFor(meta)

	firstBlock := encodeTestHPACK(t, encoder, &encoded,
		hpack.HeaderField{Name: ":method", Value: "GET"},
		hpack.HeaderField{Name: ":path", Value: "/first"},
		hpack.HeaderField{Name: ":authority", Value: "api.example.com"},
		hpack.HeaderField{Name: "x-session-hint", Value: "reused-value"},
	)
	firstFrame := testTLSHTTP2Frame(0x1, 0x4, 7, firstBlock)
	first := decoder.enrichFrame(key, tlsHTTP2FrameEvent(meta, firstFrame, 0), firstFrame)
	if first.Headers["x-session-hint"] != "reused-value" {
		t.Fatalf("first dynamic header = %q", first.Headers["x-session-hint"])
	}

	secondBlock := encodeTestHPACK(t, encoder, &encoded,
		hpack.HeaderField{Name: ":method", Value: "GET"},
		hpack.HeaderField{Name: ":path", Value: "/second"},
		hpack.HeaderField{Name: ":authority", Value: "api.example.com"},
		hpack.HeaderField{Name: "x-session-hint", Value: "reused-value"},
	)
	secondFrame := testTLSHTTP2Frame(0x1, 0x4, 9, secondBlock)
	second := decoder.enrichFrame(key, tlsHTTP2FrameEvent(meta, secondFrame, 0), secondFrame)
	if second.Type != "http2_request" || second.URL != "/second" || second.Headers["x-session-hint"] != "reused-value" {
		t.Fatalf("second dynamic-table decode failed: %+v", second)
	}
}
