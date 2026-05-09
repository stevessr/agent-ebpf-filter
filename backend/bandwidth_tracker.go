package main

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// ── Connection bandwidth tracking (per-flow byte/packet accounting) ──

type flowBytes struct {
	BytesIn    uint64
	BytesOut   uint64
	PacketsIn  uint64
	PacketsOut uint64
	FirstSeen  time.Time
	LastSeen   time.Time
	Comm       string
	PID        uint32
	PeakBpsIn  float64
	PeakBpsOut float64
}

type bandwidthTracker struct {
	mu    sync.RWMutex
	flows map[string]*flowBytes // key: "srcIP:dstIP:dstPort:protocol"
}

func newBandwidthTracker() *bandwidthTracker {
	return &bandwidthTracker{
		flows: make(map[string]*flowBytes),
	}
}

func (b *bandwidthTracker) flowKey(srcIP, dstIP string, dstPort uint32, protocol string) string {
	return fmt.Sprintf("%s:%s:%d:%s", srcIP, dstIP, dstPort, protocol)
}

func (b *bandwidthTracker) RecordBytes(srcIP, dstIP string, dstPort uint32, protocol string, direction string, bytes uint64, comm string, pid uint32) {
	if b == nil || bytes == 0 {
		return
	}
	key := b.flowKey(srcIP, dstIP, dstPort, protocol)
	now := time.Now().UTC()

	b.mu.Lock()
	defer b.mu.Unlock()

	flow, ok := b.flows[key]
	if !ok {
		flow = &flowBytes{
			FirstSeen: now,
			Comm:      comm,
			PID:       pid,
		}
		b.flows[key] = flow
	}

	switch direction {
	case "incoming":
		flow.BytesIn += bytes
		flow.PacketsIn++
	case "outgoing":
		flow.BytesOut += bytes
		flow.PacketsOut++
	}

	flow.LastSeen = now

	// Update peak bandwidth (rough estimate: bytes/second since first seen)
	elapsed := now.Sub(flow.FirstSeen).Seconds()
	if elapsed > 0 {
		bpsIn := float64(flow.BytesIn) / elapsed
		bpsOut := float64(flow.BytesOut) / elapsed
		if bpsIn > flow.PeakBpsIn {
			flow.PeakBpsIn = bpsIn
		}
		if bpsOut > flow.PeakBpsOut {
			flow.PeakBpsOut = bpsOut
		}
	}
}

func (b *bandwidthTracker) Snapshot() []flowBytes {
	b.mu.RLock()
	defer b.mu.RUnlock()

	flows := make([]flowBytes, 0, len(b.flows))
	for _, f := range b.flows {
		flows = append(flows, *f)
	}
	return flows
}

func (b *bandwidthTracker) EvictOlderThan(maxAge time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cutoff := time.Now().UTC().Add(-maxAge)
	for key, flow := range b.flows {
		if flow.LastSeen.Before(cutoff) {
			delete(b.flows, key)
		}
	}
}

var globalBandwidthTracker = newBandwidthTracker()

// ── Data exfiltration detection ───────────────────────────────────────

type ExfilAlert struct {
	FlowKey     string  `json:"flowKey"`
	DstIP       string  `json:"dstIp"`
	DstPort     uint32  `json:"dstPort"`
	DstDomain   string  `json:"dstDomain,omitempty"`
	BytesOut    uint64  `json:"bytesOut"`
	Threshold   uint64  `json:"threshold"`
	RiskScore   float64 `json:"riskScore"`
	Reason      string  `json:"reason"`
	Comm        string  `json:"comm"`
	PID         uint32  `json:"pid"`
	DetectedAt  string  `json:"detectedAt"`
}

type exfilDetector struct {
	mu sync.RWMutex
	// Thresholds
	volumeThresholdBytes uint64 // alert if a single flow exceeds this
	rateThresholdBps     float64 // alert if sustained rate exceeds this
	durationThresholdSec float64 // alert if flow duration exceeds this
	// Cooldown to avoid duplicate alerts
	lastAlertByKey map[string]time.Time
	cooldown       time.Duration
}

func newExfilDetector() *exfilDetector {
	return &exfilDetector{
		volumeThresholdBytes: 10 * 1024 * 1024, // 10 MB
		rateThresholdBps:     1 * 1024 * 1024,   // 1 MB/s
		durationThresholdSec: 300,               // 5 minutes
		lastAlertByKey:       make(map[string]time.Time),
		cooldown:             5 * time.Minute,
	}
}

func (d *exfilDetector) CheckFlow(flow flowBytes, key, dstIP, dstDomain string, dstPort uint32) *ExfilAlert {
	if d == nil {
		return nil
	}
	now := time.Now().UTC()

	// Cooldown check
	d.mu.Lock()
	if last, ok := d.lastAlertByKey[key]; ok && now.Sub(last) < d.cooldown {
		d.mu.Unlock()
		return nil
	}
	d.mu.Unlock()

	var reason string
	var risk float64

	// Volume-based detection
	if flow.BytesOut > d.volumeThresholdBytes {
		reason = fmt.Sprintf("outbound volume exceeded %.0f MB", float64(d.volumeThresholdBytes)/1024/1024)
		ratio := float64(flow.BytesOut) / float64(d.volumeThresholdBytes)
		risk = math.Min(0.50+ratio*0.10, 0.95)
	}

	// Rate-based detection
	elapsed := flow.LastSeen.Sub(flow.FirstSeen).Seconds()
	if elapsed > 0 {
		bpsOut := float64(flow.BytesOut) / elapsed
		if bpsOut > d.rateThresholdBps {
			if reason != "" {
				reason += "; "
			}
			reason += fmt.Sprintf("sustained outbound rate %.1f MB/s", bpsOut/1024/1024)
			rate := bpsOut / d.rateThresholdBps
			risk = math.Max(risk, math.Min(0.50+rate*0.15, 0.97))
		}
	}

	// Duration-based detection
	if elapsed > d.durationThresholdSec {
		if reason != "" {
			reason += "; "
		}
		reason += fmt.Sprintf("long-lived connection (%.0f min)", elapsed/60)
		risk = math.Max(risk, 0.60)
	}

	if reason == "" {
		return nil
	}

	// Record alert
	d.mu.Lock()
	d.lastAlertByKey[key] = now
	d.mu.Unlock()

	return &ExfilAlert{
		FlowKey:    key,
		DstIP:      dstIP,
		DstPort:    dstPort,
		DstDomain:  dstDomain,
		BytesOut:   flow.BytesOut,
		Threshold:  d.volumeThresholdBytes,
		RiskScore:  risk,
		Reason:     reason,
		Comm:       flow.Comm,
		PID:        flow.PID,
		DetectedAt: now.Format(time.RFC3339Nano),
	}
}

func (d *exfilDetector) RunCheck() []ExfilAlert {
	if d == nil {
		return nil
	}
	flows := globalBandwidthTracker.Snapshot()
	alerts := make([]ExfilAlert, 0)

	for _, flow := range flows {
		dstIP := ""
		dstPort := uint32(0)
		// Parse flow key: "srcIP:dstIP:dstPort:protocol"
		keyParts := splitFlowKey(flow)
		if len(keyParts) >= 3 {
			dstIP = keyParts[1]
			if p, err := parseUint32Str(keyParts[2]); err == nil {
				dstPort = p
			}
		}

		// Enrich with DNS
		dstDomain, _ := dnsCorrelation.LookupIP(dstIP)

		alert := d.CheckFlow(flow, keyJoin(flow), dstIP, dstDomain, dstPort)
		if alert != nil {
			alerts = append(alerts, *alert)
		}
	}

	return alerts
}

func splitFlowKey(flow flowBytes) []string {
	// The flowKey function above uses ":" as separator
	// We need to reconstruct from the flow data
	return nil
}

func keyJoin(flow flowBytes) string {
	return fmt.Sprintf("%s::%d:tcp", flow.Comm, flow.PID)
}

func parseUint32Str(s string) (uint32, error) {
	var result uint32
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + uint32(c-'0')
		} else {
			return 0, fmt.Errorf("not a number")
		}
	}
	return result, nil
}

var exfilDetectorInst = newExfilDetector()

// Start periodic exfiltration checks
func startExfilDetectionLoop() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			alerts := exfilDetectorInst.RunCheck()
			if len(alerts) > 0 {
				logExfilAlerts(alerts)
			}
		}
	}()
}

func logExfilAlerts(alerts []ExfilAlert) {
	for _, alert := range alerts {
		log.Printf("[EXFIL] risk=%.2f reason=%q flow=%s bytes=%s",
			alert.RiskScore, alert.Reason, alert.FlowKey, formatBytes(alert.BytesOut))
	}
}

// ── Connection history archive ────────────────────────────────────────

type archivedConnection struct {
	SrcIP      string    `json:"srcIp"`
	DstIP      string    `json:"dstIp"`
	DstPort    uint32    `json:"dstPort"`
	DstService string    `json:"dstService"`
	DstDomain  string    `json:"dstDomain"`
	BytesIn    uint64    `json:"bytesIn"`
	BytesOut   uint64    `json:"bytesOut"`
	Comm       string    `json:"comm"`
	PID        uint32    `json:"pid"`
	FirstSeen  time.Time `json:"firstSeen"`
	LastSeen   time.Time `json:"lastSeen"`
	ClosedAt   time.Time `json:"closedAt"`
	State      string    `json:"state"`
	IPScope    string    `json:"ipScope"`
}

type connectionArchive struct {
	mu       sync.RWMutex
	archived []archivedConnection
	maxSize  int
}

func newConnectionArchive(maxSize int) *connectionArchive {
	return &connectionArchive{
		archived: make([]archivedConnection, 0, maxSize),
		maxSize:  maxSize,
	}
}

func (a *connectionArchive) Archive(conn archivedConnection) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	a.archived = append(a.archived, conn)
	if len(a.archived) > a.maxSize {
		// Evict oldest
		a.archived = a.archived[len(a.archived)-a.maxSize:]
	}
}

func (a *connectionArchive) Snapshot() []archivedConnection {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]archivedConnection, len(a.archived))
	copy(result, a.archived)
	return result
}

var connectionHistory = newConnectionArchive(5000)

// ── Data volume anomaly score for semantic alerts ────────────────────

func computeExfilRiskScore(bytesOut uint64, elapsedSec float64, dstScope string) float64 {
	if bytesOut == 0 || elapsedSec <= 0 {
		return 0
	}

	bpsOut := float64(bytesOut) / elapsedSec
	risk := 0.0

	// Volume thresholds
	switch {
	case bytesOut > 100*1024*1024: // >100 MB
		risk = 0.95
	case bytesOut > 50*1024*1024: // >50 MB
		risk = 0.85
	case bytesOut > 10*1024*1024: // >10 MB
		risk = 0.65
	case bytesOut > 1*1024*1024: // >1 MB
		risk = 0.35
	}

	// Rate thresholds
	switch {
	case bpsOut > 10*1024*1024: // >10 MB/s
		risk = math.Max(risk, 0.95)
	case bpsOut > 1*1024*1024: // >1 MB/s
		risk = math.Max(risk, 0.75)
	case bpsOut > 100*1024: // >100 KB/s
		risk = math.Max(risk, 0.45)
	}

	// Scope adjustment
	if dstScope == string(ScopePublic) {
		risk = math.Min(risk*1.2, 0.99)
	}

	return risk
}

// formatBytes returns human-readable byte string
func formatBytes(bytes uint64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(bytes)/1024/1024/1024)
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// formatBps returns human-readable bandwidth string
func formatBps(bps float64) string {
	switch {
	case bps >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB/s", bps/1024/1024/1024)
	case bps >= 1024*1024:
		return fmt.Sprintf("%.1f MB/s", bps/1024/1024)
	case bps >= 1024:
		return fmt.Sprintf("%.1f KB/s", bps/1024)
	default:
		return fmt.Sprintf("%.0f B/s", bps)
	}
}
