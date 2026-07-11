package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBoundedAgentSightIDSetEvictsOldestEntries(t *testing.T) {
	set := newBoundedAgentSightIDSet(3)
	for _, id := range []string{"one", "two", "three", "four", "five"} {
		if !set.Add(id) {
			t.Fatalf("first add of %q was rejected", id)
		}
	}

	if set.Len() != 3 {
		t.Fatalf("len = %d, want 3", set.Len())
	}
	for _, id := range []string{"one", "two"} {
		if set.Contains(id) {
			t.Fatalf("oldest id %q was not evicted", id)
		}
	}
	for _, id := range []string{"three", "four", "five"} {
		if !set.Contains(id) {
			t.Fatalf("newest id %q is missing", id)
		}
	}
}

func TestBoundedAgentSightIDSetDuplicateDoesNotEvict(t *testing.T) {
	set := newBoundedAgentSightIDSet(2)
	set.Add("one")
	set.Add("two")
	if set.Add("one") {
		t.Fatal("duplicate add was accepted")
	}
	set.Add("three")

	if set.Contains("one") {
		t.Fatal("oldest id was not evicted after duplicate")
	}
	if !set.Contains("two") || !set.Contains("three") {
		t.Fatalf("set contents after duplicate/eviction are incorrect: %#v", set.entries)
	}
}

func TestAgentSightStreamDedupeCapacityIsBounded(t *testing.T) {
	if got := agentSightStreamDedupeCapacity(25); got != 50 {
		t.Fatalf("capacity = %d, want 50", got)
	}
	if got := agentSightStreamDedupeCapacity(agentSightMaxLimit * 10); got != agentSightMaxLimit*2 {
		t.Fatalf("capped capacity = %d, want %d", got, agentSightMaxLimit*2)
	}
	if got := agentSightStreamDedupeCapacity(0); got != agentSightDefaultLimit*2 {
		t.Fatalf("default capacity = %d, want %d", got, agentSightDefaultLimit*2)
	}
}

func TestHandleAgentSightEventsUploadRejectsOversizeBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/agentsight/events", HandleAgentSightEventsUpload)

	req := httptest.NewRequest(
		http.MethodPost,
		"/agentsight/events",
		strings.NewReader(strings.Repeat("x", int(AgentSightUploadMaxBytes)+1)),
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	var payload struct {
		ByteLimit int64 `json:"byteLimit"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ByteLimit != AgentSightUploadMaxBytes {
		t.Fatalf("byteLimit = %d, want %d", payload.ByteLimit, AgentSightUploadMaxBytes)
	}
}

func TestParseAgentSightUploadPayloadRejectsExcessRecords(t *testing.T) {
	_, err := parseAgentSightUploadPayload([]byte(agentSightTestJSONArray(AgentSightUploadMaxEvents + 1)))
	if !errors.Is(err, errAgentSightUploadEventLimit) {
		t.Fatalf("error = %v, want upload event limit", err)
	}
}

func TestParseAgentSightUploadPayloadRecordLimitFormats(t *testing.T) {
	exact, err := parseAgentSightUploadPayload([]byte(agentSightTestJSONArray(AgentSightUploadMaxEvents)))
	if err != nil || len(exact) != AgentSightUploadMaxEvents {
		t.Fatalf("exact-limit parse = len:%d err:%v", len(exact), err)
	}

	excessArray := agentSightTestJSONArray(AgentSightUploadMaxEvents + 1)
	for name, payload := range map[string]string{
		"events_wrapper":  `{"events":` + excessArray + `}`,
		"records_wrapper": `{"records":` + excessArray + `}`,
		"jsonl":           strings.Repeat("{\"timestamp\":1}\n", AgentSightUploadMaxEvents+1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseAgentSightUploadPayload([]byte(payload))
			if !errors.Is(err, errAgentSightUploadEventLimit) {
				t.Fatalf("error = %v, want upload event limit", err)
			}
		})
	}
}

func TestHandleAgentSightEventsUploadReportsRecordLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/agentsight/events", HandleAgentSightEventsUpload)
	req := httptest.NewRequest(
		http.MethodPost,
		"/agentsight/events",
		strings.NewReader(agentSightTestJSONArray(AgentSightUploadMaxEvents+1)),
	)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	var payload struct {
		RecordLimit int `json:"recordLimit"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.RecordLimit != AgentSightUploadMaxEvents {
		t.Fatalf("recordLimit = %d, want %d", payload.RecordLimit, AgentSightUploadMaxEvents)
	}
}

func agentSightTestJSONArray(count int) string {
	var body strings.Builder
	body.Grow(count * 16)
	body.WriteByte('[')
	for i := 0; i < count; i++ {
		if i > 0 {
			body.WriteByte(',')
		}
		body.WriteString(`{"timestamp":1}`)
	}
	body.WriteByte(']')
	return body.String()
}
