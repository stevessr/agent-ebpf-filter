package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIClientAddsTokenToRESTRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-KEY"); got != "secret" {
			t.Fatalf("X-API-KEY = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[{"eventId":"evt-1","receivedAtMs":42,"comm":"agent"}]}`))
	}))
	defer server.Close()

	client, err := newAPIClient(server.URL, "secret")
	if err != nil {
		t.Fatal(err)
	}
	events, err := client.summaries(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != "evt-1" {
		t.Fatalf("unexpected events: %#v", events)
	}
}
