package main

import (
	"context"
	"encoding/json"
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

func TestAPIClientConfigurationMutations(t *testing.T) {
	var saved wrapperRule
	deleted := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "secret" {
			t.Errorf("missing API key for %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/config/runtime":
			_, _ = w.Write([]byte(`{"runtime":{"disabledEventTypes":[1,25],"policyManagementEnabled":true},"persistedEventLogAlive":true}`))
		case r.Method == http.MethodPut && r.URL.Path == "/config/runtime":
			var patch map[string]any
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				t.Errorf("decode runtime patch: %v", err)
			}
			if _, ok := patch["disabledEventTypes"]; !ok {
				t.Errorf("runtime patch missing disabledEventTypes: %#v", patch)
			}
			_, _ = w.Write([]byte(`{"runtime":{"disabledEventTypes":[25],"policyManagementEnabled":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/config/rules":
			_, _ = w.Write([]byte(`{"curl":{"action":"BLOCK","priority":7}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/config/rules":
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Errorf("decode saved rule: %v", err)
			}
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.Method == http.MethodDelete:
			deleted = r.URL.Path
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.Error(w, "unexpected route", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := newAPIClient(server.URL, "secret")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	runtime, err := client.runtimeConfig(ctx)
	if err != nil || !runtime.PersistedEventLogAlive {
		t.Fatalf("runtimeConfig() = %#v, %v", runtime, err)
	}
	updated, err := client.patchRuntime(ctx, map[string]any{"disabledEventTypes": []uint32{25}})
	if err != nil || len(disabledEventTypeSet(updated.Runtime)) != 1 {
		t.Fatalf("patchRuntime() = %#v, %v", updated, err)
	}
	rules, err := client.rules(ctx)
	if err != nil || len(rules) != 1 || rules[0].Comm != "curl" || rules[0].Priority != 7 {
		t.Fatalf("rules() = %#v, %v", rules, err)
	}
	wantRule := wrapperRule{Comm: "echo", Action: "REWRITE", RewrittenCmd: []string{"printf", "ok"}, Priority: 3}
	if err := client.saveRule(ctx, wantRule); err != nil {
		t.Fatal(err)
	}
	if saved.Comm != wantRule.Comm || len(saved.RewrittenCmd) != 2 || saved.RewrittenCmd[1] != "ok" {
		t.Fatalf("saved rule = %#v", saved)
	}
	if err := client.deleteRule(ctx, "nc open"); err != nil {
		t.Fatal(err)
	}
	if deleted != "/config/rules/nc open" {
		t.Fatalf("delete path = %q", deleted)
	}
}
