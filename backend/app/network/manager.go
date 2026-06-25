package network

import (
	"time"

	"agent-ebpf-filter/internal/geoip"
	netcore "agent-ebpf-filter/internal/network"
)

// Manager aggregates all network-tracking state previously held in package-level
// global variables. Create via NewManager and pass to dependent code.
type Manager struct {
	dnsCorrelation    *dnsCache
	tcpTracker        *tcpStateTracker
	bandwidthTracker  *bandwidthTracker
	exfilDetectorInst *exfilDetector
	connectionHistory *connectionArchive
	geoipDB           *geoip.Resolver
}

// NewManager creates a Manager with all internal state initialized.
func NewManager() *Manager {
	return &Manager{
		dnsCorrelation:    newDNSCache(),
		tcpTracker:        newTCPStateTracker(),
		bandwidthTracker:  newBandwidthTracker(),
		exfilDetectorInst: newExfilDetector(),
		connectionHistory: newConnectionArchive(5000),
		geoipDB:           geoip.NewResolver(),
	}
}

// ── DNS cache methods ──────────────────────────────────────────────────────

// StartDNSCacheGC launches a background goroutine that evicts expired DNS cache entries.
func (m *Manager) StartDNSCacheGC() {
	startDNSCacheGC(m.dnsCorrelation)
}

// RecordDNSQueryFromEvent records a DNS query domain for later correlation.
func (m *Manager) RecordDNSQueryFromEvent(domain string) {
	recordDNSQueryFromEvent(m.dnsCorrelation, domain)
}

// CorrelateDNSResponse correlates a DNS response packet with prior queries.
func (m *Manager) CorrelateDNSResponse(srcIP string, rawData []byte) {
	correlateDNSResponse(m.dnsCorrelation, srcIP, rawData)
}

// DNSLookupIP looks up a domain by IP address from the DNS cache.
func (m *Manager) DNSLookupIP(ip string) (string, bool) {
	return m.dnsCorrelation.LookupIP(ip)
}

// DNSLookupDomain looks up IP by domain name from the DNS cache.
func (m *Manager) DNSLookupDomain(domain string) (string, bool) {
	return m.dnsCorrelation.LookupDomain(domain)
}

// DNSSnapshot returns a snapshot of the current DNS cache entries.
func (m *Manager) DNSSnapshot() []dnsCacheSnapshotEntry {
	return m.dnsCorrelation.Snapshot()
}

// DNSCache returns the underlying DNS cache (for injection into aggregators).
func (m *Manager) DNSCache() *dnsCache {
	return m.dnsCorrelation
}

// ── TCP state tracker methods ─────────────────────────────────────────────

// TCPStateFromLinux converts a Linux TCP state value to a readable constant.
func (m *Manager) TCPStateFromLinux(state uint8) TCPState {
	return tcpStateFromLinux(state)
}

// TCPConnKey returns a canonical connection key string.
func (m *Manager) TCPConnKey(srcIP, dstIP string, srcPort, dstPort uint32) string {
	return netcore.TCPConnKey(srcIP, dstIP, srcPort, dstPort)
}

// StartTCPStateTrackerGC launches a background goroutine that evicts terminal TCP states.
func (m *Manager) StartTCPStateTrackerGC() {
	startTCPStateTrackerGC(m.tcpTracker)
}

func (m *Manager) RecordTCPStateChange(srcIP, dstIP string, srcPort, dstPort uint32, oldState, newState uint8, pid uint32, comm string) {
	m.tcpTracker.RecordStateChange(srcIP, dstIP, srcPort, dstPort, oldState, newState, pid, comm)
}

func (m *Manager) RecordTCPConnect(srcIP, dstIP string, srcPort, dstPort uint32, pid uint32, comm string) {
	m.tcpTracker.RecordConnect(srcIP, dstIP, srcPort, dstPort, pid, comm)
}

func (m *Manager) RecordTCPClose(srcIP, dstIP string, srcPort, dstPort uint32) {
	m.tcpTracker.RecordClose(srcIP, dstIP, srcPort, dstPort)
}

func (m *Manager) TCPSnapshot() []tcpConnectionState {
	return m.tcpTracker.Snapshot()
}

func (m *Manager) EvictTerminalTCPStates(maxAge time.Duration) {
	m.tcpTracker.EvictTerminalOlderThan(maxAge)
}

// ── Bandwidth tracker methods ──────────────────────────────────────────────

func (m *Manager) RecordBandwidthBytes(srcIP, dstIP string, dstPort uint32, protocol, direction string, bytes uint64, comm string, pid uint32) {
	m.bandwidthTracker.RecordBytes(srcIP, dstIP, dstPort, protocol, direction, bytes, comm, pid)
}

func (m *Manager) BandwidthSnapshot() []flowBytes {
	return m.bandwidthTracker.Snapshot()
}

func (m *Manager) EvictBandwidthOlderThan(maxAge time.Duration) {
	m.bandwidthTracker.EvictOlderThan(maxAge)
}

// ── Exfiltration detection methods ─────────────────────────────────────────

// StartExfilDetectionLoop launches a background goroutine that periodically checks for exfiltration.
func (m *Manager) StartExfilDetectionLoop() {
	startExfilDetectionLoop(m.bandwidthTracker, m.dnsCorrelation, m.exfilDetectorInst)
}

// ── Connection history methods ─────────────────────────────────────────────

func (m *Manager) ArchiveConnection(conn archivedConnection) {
	m.connectionHistory.Archive(conn)
}

func (m *Manager) ConnectionHistorySnapshot() []archivedConnection {
	return m.connectionHistory.Snapshot()
}

// ── GeoIP methods ──────────────────────────────────────────────────────────

func (m *Manager) InitGeoIPDatabase() {
	initGeoIPDatabase()
}

func (m *Manager) EnrichEndpointWithGeoIP(endpoint string) string {
	return enrichEndpointWithGeoIP(m.geoipDB, endpoint)
}

// LookupService is a stateless helper — exposed here for convenience.
func LookupService(port uint16) string {
	return lookupService(port)
}

func LookupServiceByPort(port uint32) string {
	return lookupServiceByPort(port)
}

func IsSuspiciousPortService(serviceName string) bool {
	return isSuspiciousPortService(serviceName)
}

// StartGC launches all background GC goroutines for network tracking state.
func (m *Manager) StartGC() {
	m.StartDNSCacheGC()
	m.StartTCPStateTrackerGC()
	m.StartExfilDetectionLoop()
}

// ── Helpers (stateless) ────────────────────────────────────────────────────

func ComputeExfilRiskScore(bytesOut uint64, elapsedSec float64, dstScope string) float64 {
	return computeExfilRiskScore(bytesOut, elapsedSec, dstScope)
}

func FormatBytes(bytes uint64) string {
	return formatBytes(bytes)
}

func FormatBps(bps float64) string {
	return formatBps(bps)
}