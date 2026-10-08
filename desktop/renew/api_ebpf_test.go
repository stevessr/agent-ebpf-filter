package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestEBPFModulesUsesRegisteredPluginState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/plugins" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-API-KEY") != "local-token" {
			t.Error("missing local backend token")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"plugins": []map[string]any{
			{"id": "web-hook", "kind": "webhook", "loaded": true},
			{"id": "z-trace", "name": "trace", "kind": "ebpf", "enabled": true, "loaded": true, "attachKind": "tracepoint", "attachTarget": "syscalls/sys_enter_execve"},
			{"id": "a-lsm", "kind": "ebpf", "loaded": false, "loadError": "permission denied"},
		}})
	}))
	defer server.Close()

	client := newAPIClient(server.URL, "local-token")
	modules, err := client.ebpfModules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ids := []string{modules[0].ID, modules[1].ID}; !reflect.DeepEqual(ids, []string{"a-lsm", "z-trace"}) {
		t.Fatalf("filtered/sorted module IDs = %v", ids)
	}
	if modules[0].LoadError != "permission denied" || !modules[1].Loaded || !modules[1].Enabled || modules[1].AttachKind != "tracepoint" {
		t.Fatalf("runtime fields lost: %+v", modules)
	}
}

func TestEBPFModuleMutationsUseBackendLifecycleRoutes(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-token" || r.Header.Get("X-API-KEY") != "local-token" {
			t.Error("missing authorization")
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %s", r.Method)
		}
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.ID != "sample-trace" {
			t.Errorf("unexpected target ID %q", body.ID)
		}
		requests = append(requests, r.URL.Path)
		loaded := r.URL.Path == "/plugins/bpf/load"
		_ = json.NewEncoder(w).Encode(map[string]any{"plugin": map[string]any{
			"id": body.ID, "kind": "ebpf", "loaded": loaded,
		}})
	}))
	defer server.Close()

	client := newAPIClient(server.URL, "local-token")
	for _, load := range []bool{true, false} {
		module, err := client.setEBPFModuleLoaded(context.Background(), "sample-trace", load)
		if err != nil {
			t.Fatal(err)
		}
		if module.Loaded != load {
			t.Fatalf("loaded=%v, want %v", module.Loaded, load)
		}
	}
	if !reflect.DeepEqual(requests, []string{"/plugins/bpf/load", "/plugins/bpf/unload"}) {
		t.Fatalf("lifecycle routes = %v", requests)
	}
}

func TestEBPFModuleMutationRejectsUnconfirmedStateAndBackendFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plugins/bpf/load":
			_ = json.NewEncoder(w).Encode(map[string]any{"plugin": map[string]any{
				"id": "sample-trace", "kind": "ebpf", "loaded": false,
			}})
		case "/plugins/bpf/unload":
			http.Error(w, `{"error":"policy denied"}`, http.StatusForbidden)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newAPIClient(server.URL, "")
	if _, err := client.setEBPFModuleLoaded(context.Background(), "", true); err == nil {
		t.Fatal("empty IDs must be rejected")
	}
	if _, err := client.setEBPFModuleLoaded(context.Background(), "sample-trace", true); err == nil || !strings.Contains(err.Error(), "did not confirm") {
		t.Fatalf("unconfirmed load should fail, got %v", err)
	}
	if _, err := client.setEBPFModuleLoaded(context.Background(), "sample-trace", false); err == nil {
		t.Fatal("backend refusal must not look successful")
	}
}

func TestFindEBPFModule(t *testing.T) {
	modules := []ebpfModule{{ID: "alpha", Loaded: true}, {ID: "beta"}}
	if got, ok := findEBPFModule(modules, "alpha"); !ok || !got.Loaded {
		t.Fatal("missing loaded module")
	}
	if _, ok := findEBPFModule(modules, "missing"); ok {
		t.Fatal("unexpected unknown module")
	}
}
