package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNormalizeOrigin(t *testing.T) {
	for _, test := range []struct {
		address string
		ok      bool
	}{
		{"https://monitor.example.com:443", true},
		{"http://127.0.0.1:8080", true},
		{"http://[::1]:8080", true},
		{"https://monitor.example.com/", false},
		{"http://192.168.1.10:8080", false},
		{"http://example.com", false},
		{"ftp://monitor.example.com", false},
		{"https://user:secret@monitor.example.com", false},
		{"https://monitor.example.com/private", false},
		{"https://monitor.example.com?token=x", false},
		{"https://monitor.example.com#fragment", false},
		{"https://monitor.example.com:99999", false},
		{"", false},
	} {
		_, err := NormalizeOrigin(test.address)
		if (err == nil) != test.ok {
			t.Errorf("%q: success=%t want %t, err=%v", test.address, err == nil, test.ok, err)
		}
	}
}

func TestFetchSnapshotUsesReadOnlyAuthenticatedAPI(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.Header.Get("X-API-KEY") != "test-token" ||
			r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected method/auth: %s %v", r.Method, r.Header)
			http.Error(w, "bad auth", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/system/collector-health":
			fmt.Fprint(w, `{"captureHealthy":true,"ringbufDroppedTotal":3,"backendQueueLen":2}`)
		case "/events/summaries":
			if r.URL.Query().Get("compact") != "1" || r.URL.Query().Get("limit") != "60" {
				t.Errorf("bad query: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"events":[{"eventId":"e1","pid":42,"tag":"agent","decision":"BLOCK","riskScore":0.9}],"nextCursor":"next"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-token")
	if err != nil { t.Fatal(err) }
	result, err := client.FetchSnapshot(context.Background(), 200)
	if err != nil { t.Fatal(err) }
	if calls.Load() != 2 || !result.Health.CaptureHealthy || result.Health.RingbufDroppedTotal != 3 ||
		len(result.Events) != 1 || result.Events[0].EventID != "e1" {
		t.Fatalf("bad snapshot: %+v (requests=%d)", result, calls.Load())
	}
}

func TestFetchRejectsAuthFailureAndRedirects(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/system/collector-health" {
			http.Redirect(w, r, target.URL+"/stolen", http.StatusFound)
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "secret-token")
	if _, err := client.FetchSnapshot(context.Background(), 2); err == nil || !strings.Contains(err.Error(), "302") {
		t.Fatalf("redirect should fail without following: %v", err)
	}
	if redirected.Load() != 0 { t.Fatal("credential-bearing redirect was followed") }
}

func TestFetchRejectsOversizedAndInvalidJSON(t *testing.T) {
	for _, body := range []string{strings.Repeat("x", maxResponseBytes+1), "not JSON"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, body)
		}))
		client, _ := NewClient(server.URL, "")
		_, err := client.FetchSnapshot(context.Background(), 20)
		server.Close()
		if err == nil { t.Fatalf("body len %d should fail", len(body)) }
	}
}

func TestFetchEventDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events/detail/abc-123" { t.Errorf("wrong path: %q", r.URL.Path) }
		fmt.Fprint(w, `{"eventId":"abc-123","decision":"BLOCK"}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "")
	for _, id := range []string{"", "abc/def", "abc?x", strings.Repeat("x", 257)} {
		if _, err := client.FetchEventDetail(context.Background(), id); err == nil {
			t.Errorf("accepted %q", id)
		}
	}
	record, err := client.FetchEventDetail(context.Background(), "abc-123")
	if err != nil { t.Fatal(err) }
	encoded, _ := json.Marshal(record)
	if !strings.Contains(string(encoded), "BLOCK") { t.Fatalf("bad detail: %s", encoded) }
}
