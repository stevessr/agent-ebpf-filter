package network

import (
	"testing"
	"time"
)

func TestManagerBackgroundWorkersEvictStateAndClose(t *testing.T) {
	manager := NewManager()
	manager.RecordTCPConnect("10.0.0.2", "93.184.216.34", 41000, 443, 42, "curl")
	manager.RecordTCPClose("10.0.0.2", "93.184.216.34", 41000, 443)
	manager.RecordBandwidthBytes("10.0.0.2", "93.184.216.34", 443, "TCP", "outgoing", 128, "curl", 42)

	manager.startGCWithIntervals(networkGCIntervals{
		dnsSweep:          time.Hour,
		tcpSweep:          time.Millisecond,
		tcpTerminalMaxAge: -time.Nanosecond,
		exfilSweep:        time.Hour,
		bandwidthSweep:    time.Millisecond,
		bandwidthMaxAge:   -time.Nanosecond,
	})
	// Starting through any compatibility entrypoint must remain idempotent.
	manager.StartGC()
	manager.StartDNSCacheGC()

	deadline := time.Now().Add(time.Second)
	for len(manager.TCPSnapshot()) != 0 || len(manager.BandwidthSnapshot()) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("background eviction did not converge: tcp=%d bandwidth=%d", len(manager.TCPSnapshot()), len(manager.BandwidthSnapshot()))
		}
		time.Sleep(time.Millisecond)
	}

	closed := make(chan struct{})
	go func() {
		manager.Close()
		manager.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Manager.Close did not stop background workers")
	}

	manager.StartGC()
	manager.lifecycleMu.Lock()
	isClosed := manager.closed
	manager.lifecycleMu.Unlock()
	if !isClosed {
		t.Fatal("manager was restarted after Close")
	}
}
