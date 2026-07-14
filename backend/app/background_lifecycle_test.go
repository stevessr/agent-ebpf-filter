package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/pb"
)

func TestRunCgroupAttributionGCStopsWithContext(t *testing.T) {
	store := newCgroupAttributionStore()
	store.Set(42, cgroupAttributionEntry{
		AgentRunID: "run-old",
		CreatedAt:  time.Now().UTC().Add(-time.Hour),
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runCgroupAttributionGC(ctx, store, time.Millisecond, 30*time.Minute)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if _, ok := store.Get(42); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cgroup attribution GC did not evict stale state")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cgroup attribution GC did not stop after cancellation")
	}
}

func TestRunArchiveEvictionLoopStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runArchiveEvictionLoop(ctx, newEventArchive(8), time.Hour)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("archive eviction loop did not stop after cancellation")
	}
}

func TestRunSemanticAlertStateGCStopsWithContext(t *testing.T) {
	state := events.NewSemanticAlertState()
	state.RememberSecret(
		&pb.Event{ToolCallId: "stale-semantic-context"},
		"/tmp/old-secret",
		time.Now().UTC().Add(-events.SemanticSecretCorrelationTTL-time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runSemanticAlertStateGC(ctx, state, time.Millisecond)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for state.Status().Entries != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("semantic state GC did not evict stale entry: %+v", state.Status())
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("semantic state GC did not stop after cancellation")
	}
	if state.Status().LastSweepAt.IsZero() {
		t.Fatal("semantic state GC did not record its sweep time")
	}
}

func TestRunToolBaselineGCStopsWithContext(t *testing.T) {
	state := newToolBaselineStore()
	state.observeAt(
		"stale-tool",
		"stale-comm",
		"execve",
		time.Now().UTC().Add(-toolBaselineTTL-time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runToolBaselineGC(ctx, state, time.Millisecond)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for state.Status().Samples != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("tool baseline GC did not evict stale sample: %+v", state.Status())
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("tool baseline GC did not stop after cancellation")
	}
	if state.Status().LastSweepAt.IsZero() {
		t.Fatal("tool baseline GC did not record its sweep time")
	}
}

func TestRunClusterHeartbeatLoopStopsWithContext(t *testing.T) {
	cfg := ClusterConfig{
		Role:      ClusterRoleSlave,
		MasterURL: "https://master.test",
		NodeURL:   "https://worker.test",
		NodeID:    "worker-1",
		NodeName:  "Worker One",
		Account:   "cluster-account",
		Password:  "cluster-password",
	}
	manager := newClusterManager(cfg)

	var calls atomic.Int32
	var capturedMu sync.Mutex
	var capturedURL, capturedAccount, capturedPassword string
	var capturedBody []byte
	client := &http.Client{Transport: testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		capturedMu.Lock()
		capturedURL = req.URL.String()
		capturedAccount = req.Header.Get(clusterAccountHeader)
		capturedPassword = req.Header.Get(clusterPasswordHeader)
		capturedBody = append(capturedBody[:0], body...)
		capturedMu.Unlock()
		calls.Add(1)
		return newTestHTTPResponse(req, http.StatusOK, "application/json", `{}`), nil
	})}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runClusterHeartbeatLoop(ctx, manager, cfg, client, 2*time.Millisecond)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for calls.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("heartbeat calls = %d, want at least 2", calls.Load())
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cluster heartbeat loop did not stop after cancellation")
	}
	stoppedAt := calls.Load()
	time.Sleep(10 * time.Millisecond)
	if calls.Load() != stoppedAt {
		t.Fatalf("heartbeat calls continued after stop: %d -> %d", stoppedAt, calls.Load())
	}

	capturedMu.Lock()
	defer capturedMu.Unlock()
	if capturedURL != "https://master.test/cluster/heartbeat" || capturedAccount != cfg.Account || capturedPassword != cfg.Password {
		t.Fatalf("heartbeat request = url:%q account:%q password:%q", capturedURL, capturedAccount, capturedPassword)
	}
	var heartbeat ClusterHeartbeatRequest
	if err := json.Unmarshal(capturedBody, &heartbeat); err != nil {
		t.Fatalf("decode heartbeat: %v", err)
	}
	if heartbeat.NodeID != cfg.NodeID || heartbeat.NodeURL != cfg.NodeURL || heartbeat.Role != ClusterRoleSlave {
		t.Fatalf("heartbeat payload = %#v", heartbeat)
	}
}
