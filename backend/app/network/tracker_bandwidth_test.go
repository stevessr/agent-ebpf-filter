package network

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func TestManagerRunExfilDetectionPreservesFlowDestination(t *testing.T) {
	manager := NewManager()
	manager.exfilDetectorInst.volumeThresholdBytes = 100
	manager.exfilDetectorInst.rateThresholdBps = math.MaxFloat64
	manager.exfilDetectorInst.durationThresholdSec = math.MaxFloat64
	manager.DNSCache().Record("uploads.example.test", "2606:4700:4700::1111")

	manager.RecordBandwidthBytes(
		"2001:4860:4860::8888",
		"2606:4700:4700::1111",
		8443,
		"TCP",
		"outgoing",
		101,
		"curl",
		77,
	)

	flows := manager.BandwidthSnapshot()
	if len(flows) != 1 {
		t.Fatalf("bandwidth flows = %d, want 1", len(flows))
	}
	flow := flows[0]
	if flow.SrcIP != "2001:4860:4860::8888" || flow.DstIP != "2606:4700:4700::1111" || flow.DstPort != 8443 || flow.Protocol != "TCP" {
		t.Fatalf("flow metadata = %#v", flow)
	}

	alerts := manager.RunExfilDetection()
	if len(alerts) != 1 {
		t.Fatalf("exfil alerts = %#v, want one", alerts)
	}
	alert := alerts[0]
	if alert.FlowKey != flow.FlowKey || alert.DstIP != flow.DstIP || alert.DstPort != flow.DstPort || alert.DstDomain != "uploads.example.test" {
		t.Fatalf("alert destination metadata = %#v, flow = %#v", alert, flow)
	}
	if alert.Comm != "curl" || alert.PID != 77 || !strings.Contains(alert.Reason, "outbound volume") {
		t.Fatalf("alert attribution = %#v", alert)
	}
	if repeated := manager.RunExfilDetection(); len(repeated) != 0 {
		t.Fatalf("cooldown emitted duplicate alerts: %#v", repeated)
	}
}

func TestExfilDetectorBoundsAlertCooldownCache(t *testing.T) {
	detector := newExfilDetector()
	detector.volumeThresholdBytes = 1
	detector.rateThresholdBps = math.MaxFloat64
	detector.durationThresholdSec = math.MaxFloat64
	old := time.Now().UTC().Add(-detector.cooldown - time.Second)
	for i := 0; i < exfilAlertCacheMaxEntries; i++ {
		detector.lastAlertByKey[fmt.Sprintf("old-%d", i)] = old
	}

	now := time.Now().UTC()
	alert := detector.CheckFlow(flowBytes{
		BytesOut:  2,
		FirstSeen: now.Add(-time.Second),
		LastSeen:  now,
	}, "new-flow", "203.0.113.1", "", 443)
	if alert == nil {
		t.Fatal("expected exfiltration alert")
	}
	if got := len(detector.lastAlertByKey); got > exfilAlertCacheMaxEntries {
		t.Fatalf("alert cooldown entries = %d, max = %d", got, exfilAlertCacheMaxEntries)
	}
	if _, ok := detector.lastAlertByKey["new-flow"]; !ok {
		t.Fatal("new alert cooldown entry was not recorded")
	}
}

func TestBandwidthTrackerRejectsUnknownDirection(t *testing.T) {
	tracker := newBandwidthTracker()
	tracker.RecordBytes("10.0.0.2", "93.184.216.34", 443, "TCP", "sideways", 512, "curl", 42)
	if flows := tracker.Snapshot(); len(flows) != 0 {
		t.Fatalf("unknown direction created flows: %#v", flows)
	}
}
