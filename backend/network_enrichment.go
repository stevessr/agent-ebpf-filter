package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── IP Address Classification (from rustnet bogon.rs) ─────────────────

type IPScope string

const (
	ScopePublic        IPScope = "Public"
	ScopeLoopback      IPScope = "Loopback"
	ScopePrivate       IPScope = "Private"
	ScopeLinkLocal     IPScope = "Link-Local"
	ScopeCGNAT         IPScope = "CGNAT"
	ScopeMulticast     IPScope = "Multicast"
	ScopeBroadcast     IPScope = "Broadcast"
	ScopeDocumentation IPScope = "Documentation"
	ScopeBenchmarking  IPScope = "Benchmarking"
	ScopeUnspecified   IPScope = "Unspecified"
	ScopeReserved      IPScope = "Reserved"
	ScopeUniqueLocal   IPScope = "Unique-Local"
	ScopeDiscard       IPScope = "Discard"
	ScopeIPv4Mapped    IPScope = "IPv4-Mapped"
	ScopeUnknown       IPScope = "Unknown"
)

func classifyIPScope(ip net.IP) IPScope {
	if ip == nil {
		return ScopeUnknown
	}
	if ip4 := ip.To4(); ip4 != nil {
		return classifyIPv4Scope(ip4)
	}
	return classifyIPv6Scope(ip.To16())
}

func classifyIPv4Scope(ip net.IP) IPScope {
	if ip == nil || len(ip) < 4 {
		return ScopeUnknown
	}
	b0, b1 := ip[0], ip[1]

	// 0.0.0.0/8 - "This host on this network"
	if b0 == 0 {
		if ip.Equal(net.IPv4zero) {
			return ScopeUnspecified
		}
		return ScopeReserved
	}

	// 0.0.0.0/32
	if ip.Equal(net.IPv4zero) {
		return ScopeUnspecified
	}

	// 10.0.0.0/8 - Private (RFC 1918)
	if b0 == 10 {
		return ScopePrivate
	}

	// 100.64.0.0/10 - CGNAT (RFC 6598)
	if b0 == 100 && b1 >= 64 && b1 <= 127 {
		return ScopeCGNAT
	}

	// 127.0.0.0/8 - Loopback
	if b0 == 127 {
		return ScopeLoopback
	}

	// 169.254.0.0/16 - Link-Local (APIPA)
	if b0 == 169 && b1 == 254 {
		return ScopeLinkLocal
	}

	// 172.16.0.0/12 - Private (RFC 1918)
	if b0 == 172 && b1 >= 16 && b1 <= 31 {
		return ScopePrivate
	}

	// 192.0.0.0/24 - IETF Protocol Assignments
	if b0 == 192 && b1 == 0 && ip[2] == 0 {
		return ScopeReserved
	}

	// 192.0.2.0/24 - Documentation (TEST-NET-1, RFC 5737)
	if b0 == 192 && b1 == 0 && ip[2] == 2 {
		return ScopeDocumentation
	}

	// 192.88.99.0/24 - 6to4 Relay
	if b0 == 192 && b1 == 88 && ip[2] == 99 {
		return ScopeReserved
	}

	// 192.168.0.0/16 - Private (RFC 1918)
	if b0 == 192 && b1 == 168 {
		return ScopePrivate
	}

	// 198.18.0.0/15 - Benchmarking (RFC 2544)
	if b0 == 198 && (b1 == 18 || b1 == 19) {
		return ScopeBenchmarking
	}

	// 198.51.100.0/24 - Documentation (TEST-NET-2, RFC 5737)
	if b0 == 198 && b1 == 51 && ip[2] == 100 {
		return ScopeDocumentation
	}

	// 203.0.113.0/24 - Documentation (TEST-NET-3, RFC 5737)
	if b0 == 203 && b1 == 0 && ip[2] == 113 {
		return ScopeDocumentation
	}

	// 224.0.0.0/4 - Multicast
	if b0 >= 224 && b0 <= 239 {
		return ScopeMulticast
	}

	// 240.0.0.0/4 - Reserved (former Class E)
	if b0 >= 240 && b0 <= 254 {
		return ScopeReserved
	}

	// 255.255.255.255/32 - Limited Broadcast
	if ip.Equal(net.IPv4bcast) {
		return ScopeBroadcast
	}

	return ScopePublic
}

func classifyIPv6Scope(ip net.IP) IPScope {
	if ip == nil || len(ip) < 16 {
		return ScopeUnknown
	}

	// ::1/128 - Loopback
	if ip.Equal(net.IPv6loopback) {
		return ScopeLoopback
	}

	// ::/128 - Unspecified
	if ip.Equal(net.IPv6zero) {
		return ScopeUnspecified
	}

	// ::ffff:0:0/96 - IPv4-mapped
	if strings.HasPrefix(ip.String(), "::ffff:") {
		ip4 := net.IPv4(ip[12], ip[13], ip[14], ip[15])
		return classifyIPv4Scope(ip4)
	}

	// fe80::/10 - Link-Local
	if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
		return ScopeLinkLocal
	}

	// fc00::/7 - Unique Local (ULA)
	if ip[0] == 0xfc || ip[0] == 0xfd {
		return ScopeUniqueLocal
	}

	// ff00::/8 - Multicast
	if ip[0] == 0xff {
		return ScopeMulticast
	}

	// 2001:db8::/32 - Documentation
	if ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
		return ScopeDocumentation
	}

	// 2001::/32 - Teredo / 2002::/16 - 6to4
	if ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x00 && ip[3] == 0x00 {
		return ScopeReserved
	}
	if ip[0] == 0x20 && ip[1] == 0x02 {
		return ScopeReserved
	}

	// 0100::/64 - Discard (RFC 6666)
	if ip[0] == 0x01 && ip[1] == 0x00 && ip[2] == 0x00 && ip[3] == 0x00 &&
		ip[4] == 0x00 && ip[5] == 0x00 && ip[6] == 0x00 && ip[7] == 0x00 {
		return ScopeDiscard
	}

	return ScopePublic
}

func ipScopeIsSuspicious(scope IPScope) bool {
	switch scope {
	case ScopeMulticast, ScopeBroadcast, ScopeReserved, ScopeDiscard, ScopeBenchmarking:
		return true
	default:
		return false
	}
}

func ipScopeRiskScore(scope IPScope) float64 {
	switch scope {
	case ScopeLoopback:
		return 0.0
	case ScopePrivate, ScopeLinkLocal, ScopeUniqueLocal:
		return 0.05
	case ScopeCGNAT:
		return 0.15
	case ScopeDocumentation, ScopeBenchmarking:
		return 0.85
	case ScopeMulticast, ScopeBroadcast:
		return 0.70
	case ScopeReserved, ScopeDiscard:
		return 0.90
	case ScopeIPv4Mapped:
		return 0.30
	case ScopePublic:
		return 0.10
	default:
		return 0.40
	}
}

// ── DNS → IP Correlation Cache (rustnet reverse-DNS concept) ─────────

type dnsEntry struct {
	Domain    string
	IP        string
	ResolvedAt time.Time
	TTL       time.Duration
}

type dnsCache struct {
	mu       sync.RWMutex
	entries  map[string]*dnsEntry // IP -> domain mapping
	byDomain map[string]string    // domain -> IP reverse mapping
}

func newDNSCache() *dnsCache {
	return &dnsCache{
		entries:  make(map[string]*dnsEntry),
		byDomain: make(map[string]string),
	}
}

func (c *dnsCache) Record(domain, ip string) {
	if c == nil || domain == "" || ip == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict old entry for this IP if it exists
	if old, ok := c.entries[ip]; ok {
		delete(c.byDomain, old.Domain)
	}

	entry := &dnsEntry{
		Domain:     domain,
		IP:         ip,
		ResolvedAt: time.Now().UTC(),
		TTL:        5 * time.Minute,
	}
	c.entries[ip] = entry
	c.byDomain[domain] = ip
}

func (c *dnsCache) LookupIP(ip string) (string, bool) {
	if c == nil || ip == "" {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[ip]
	if !ok {
		return "", false
	}
	if time.Since(entry.ResolvedAt) > entry.TTL {
		return "", false
	}
	return entry.Domain, true
}

func (c *dnsCache) LookupDomain(domain string) (string, bool) {
	if c == nil || domain == "" {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	ip, ok := c.byDomain[domain]
	if !ok {
		return "", false
	}
	entry, ok := c.entries[ip]
	if !ok || time.Since(entry.ResolvedAt) > entry.TTL {
		return "", false
	}
	return ip, true
}

func (c *dnsCache) EnrichEndpoint(endpoint string) string {
	if c == nil || endpoint == "" {
		return endpoint
	}
	// Extract IP from "ip:port" format
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}
	if domain, ok := c.LookupIP(host); ok {
		if port != "" {
			return net.JoinHostPort(domain, port)
		}
		return domain
	}
	return endpoint
}

func (c *dnsCache) EvictExpired() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for ip, entry := range c.entries {
		if time.Since(entry.ResolvedAt) > entry.TTL {
			delete(c.byDomain, entry.Domain)
			delete(c.entries, ip)
		}
	}
}

var dnsCorrelation = newDNSCache()

func startDNSCacheGC() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			dnsCorrelation.EvictExpired()
		}
	}()
}

// Process a detected DNS query and record the domain
func recordDNSQueryFromEvent(domain string) {
	if domain == "" {
		return
	}
	// Domain names from eBPF may be raw; perform basic validation
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" || len(domain) > 253 {
		return
	}
	dnsCorrelation.Record(domain, "") // IP will be filled from DNS response
}

// Correlate a DNS response with the query
func correlateDNSResponse(srcIP string, rawData []byte) {
	// For simplicity, we parse basic A-record responses
	if len(rawData) < 20 {
		return
	}
	// DNS response: flags at offset 2-3, answer count at offset 6-7
	flags := uint16(rawData[2])<<8 | uint16(rawData[3])
	if flags&0x8000 == 0 {
		return // Not a response
	}
	ancount := int(rawData[6])<<8 | int(rawData[7])
	if ancount == 0 {
		return
	}
	// Skip header (12) + question section to get to answer
	// For now: simple placeholder - in production, use net/dnsmessage
}

// ── Service Name Resolution (rustnet services.rs) ────────────────────

var portToService = map[uint16]string{
	20: "FTP-data", 21: "FTP", 22: "SSH", 23: "Telnet",
	25: "SMTP", 53: "DNS", 67: "DHCP-server", 68: "DHCP-client",
	69: "TFTP", 80: "HTTP", 88: "Kerberos", 110: "POP3",
	119: "NNTP", 123: "NTP", 135: "MS-RPC", 137: "NetBIOS-NS",
	138: "NetBIOS-DGM", 139: "NetBIOS-SSN", 143: "IMAP",
	161: "SNMP", 162: "SNMP-trap", 179: "BGP", 194: "IRC",
	389: "LDAP", 443: "HTTPS", 445: "SMB", 465: "SMTPS",
	500: "ISAKMP", 514: "Syslog", 515: "LPD", 520: "RIP",
	546: "DHCPv6-client", 547: "DHCPv6-server", 587: "SMTP-submission",
	631: "IPP", 636: "LDAPS", 853: "DNS-over-TLS", 873: "rsync",
	993: "IMAPS", 995: "POP3S", 1080: "SOCKS", 1194: "OpenVPN",
	1433: "MS-SQL", 1521: "Oracle", 1723: "PPTP", 1883: "MQTT",
	2049: "NFS", 2181: "ZooKeeper", 2375: "Docker", 2376: "Docker-TLS",
	3000: "Grafana", 3128: "Squid", 3306: "MySQL", 3389: "RDP",
	3478: "STUN", 4000: "Jitsi", 4200: "ShellInABox", 4242: "Spark",
	4369: "Erlang-EPMD", 4444: "Metasploit", 4500: "IPsec-NAT-T",
	5000: "UPnP", 5044: "Logstash", 5060: "SIP", 5222: "XMPP",
	5353: "mDNS", 5432: "PostgreSQL", 5555: "Android-ADB",
	5601: "Kibana", 5672: "AMQP", 5900: "VNC", 5984: "CouchDB",
	6000: "X11", 6379: "Redis", 6443: "k8s-api", 6667: "IRC-SSL",
	6881: "BitTorrent", 8000: "HTTP-alt", 8080: "HTTP-proxy",
	8443: "HTTPS-alt", 8888: "HTTP-alt2", 9000: "SonarQube",
	9090: "Prometheus", 9092: "Kafka", 9100: "Node-Exporter",
	9200: "Elasticsearch", 9300: "Elasticsearch-transport",
	9418: "Git", 9999: "Legacy-backdoor", 11211: "Memcached",
	15672: "RabbitMQ-mgmt", 27017: "MongoDB", 27018: "MongoDB-shard",
	31337: "BackOrifice", 50000: "SAP", 50070: "Hadoop-DFS",
}

var suspiciousPortServices = map[string]bool{
	"Metasploit": true, "BackOrifice": true, "Legacy-backdoor": true,
	"Android-ADB": true, "ShellInABox": true,
}

func lookupService(port uint16) string {
	if name, ok := portToService[port]; ok {
		return name
	}
	return ""
}

func lookupServiceByPort(port uint32) string {
	return lookupService(uint16(port))
}

func isSuspiciousPortService(serviceName string) bool {
	return suspiciousPortServices[serviceName]
}

// ── Network Flow Summary ─────────────────────────────────────────────

type NetworkFlowSummary struct {
	Protocol     string  `json:"protocol"`
	SrcIP        string  `json:"srcIp"`
	SrcPort      uint32  `json:"srcPort"`
	DstIP        string  `json:"dstIp"`
	DstPort      uint32  `json:"dstPort"`
	DstService   string  `json:"dstService,omitempty"`
	DstDomain    string  `json:"dstDomain,omitempty"`
	IPScope      string  `json:"ipScope"`
	Direction    string  `json:"direction"`
	State        string  `json:"state,omitempty"`
	BytesIn      uint64  `json:"bytesIn"`
	BytesOut     uint64  `json:"bytesOut"`
	PacketsIn    uint64  `json:"packetsIn"`
	PacketsOut   uint64  `json:"packetsOut"`
	ProcessPIDs  []uint32 `json:"processPids"`
	ProcessComms []string `json:"processComms"`
	FirstSeen    int64   `json:"firstSeen"`
	LastSeen     int64   `json:"lastSeen"`
	RiskScore    float64 `json:"riskScore"`
	AppProtocol  string  `json:"appProtocol,omitempty"`
}

type flowKey struct {
	Protocol string
	SrcIP    string
	DstIP    string
	DstPort  uint32
}

func makeFlowKey(srcIP, dstIP string, dstPort uint32, protocol string) flowKey {
	return flowKey{
		Protocol: protocol,
		SrcIP:    srcIP,
		DstIP:    dstIP,
		DstPort:  dstPort,
	}
}

type flowAggregator struct {
	mu    sync.RWMutex
	flows map[flowKey]*NetworkFlowSummary
}

func newFlowAggregator() *flowAggregator {
	return &flowAggregator{
		flows: make(map[flowKey]*NetworkFlowSummary),
	}
}

func (f *flowAggregator) RecordConnection(srcIP, dstIP string, srcPort, dstPort uint32, protocol, comm string, pid uint32, direction string, state string) {
	if f == nil {
		return
	}
	key := makeFlowKey(srcIP, dstIP, dstPort, protocol)
	now := time.Now().UTC().UnixMilli()

	f.mu.Lock()
	defer f.mu.Unlock()

	flow, ok := f.flows[key]
	if !ok {
		scope := classifyIPScope(net.ParseIP(dstIP))
		service := lookupService(uint16(dstPort))
		domain, _ := dnsCorrelation.LookupIP(dstIP)
		risk := ipScopeRiskScore(scope)
		if isSuspiciousPortService(service) {
			risk = maxFloat64(risk, 0.80)
		}

		flow = &NetworkFlowSummary{
			Protocol:    protocol,
			SrcIP:       srcIP,
			SrcPort:     srcPort,
			DstIP:       dstIP,
			DstPort:     dstPort,
			DstService:  service,
			DstDomain:   domain,
			IPScope:     string(scope),
			Direction:   direction,
			State:       state,
			ProcessPIDs: make([]uint32, 0),
			ProcessComms: make([]string, 0),
			FirstSeen:   now,
			RiskScore:   risk,
		}
		f.flows[key] = flow
	}

	flow.LastSeen = now
	if state != "" {
		flow.State = state
	}

	// Deduplicate processes
	pidExists := false
	for _, p := range flow.ProcessPIDs {
		if p == pid {
			pidExists = true
			break
		}
	}
	if !pidExists && pid > 0 {
		flow.ProcessPIDs = append(flow.ProcessPIDs, pid)
		flow.ProcessComms = append(flow.ProcessComms, comm)
	}
}

func (f *flowAggregator) Snapshot() []NetworkFlowSummary {
	f.mu.RLock()
	defer f.mu.RUnlock()

	flows := make([]NetworkFlowSummary, 0, len(f.flows))
	for _, flow := range f.flows {
		flows = append(flows, *flow)
	}
	return flows
}

func (f *flowAggregator) EvictOlderThan(maxAge time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cutoff := time.Now().UTC().Add(-maxAge).UnixMilli()
	for key, flow := range f.flows {
		if flow.LastSeen < cutoff {
			delete(f.flows, key)
		}
	}
}

var networkFlowAggregator = newFlowAggregator()

func startFlowAggregatorGC() {
	ticker := time.NewTicker(2 * time.Minute)
	go func() {
		for range ticker.C {
			networkFlowAggregator.EvictOlderThan(10 * time.Minute)
		}
	}()
}

// ── TCP State Machine (RFC 793, from rustnet) ────────────────────────

type TCPState uint8

const (
	TCPStateUnknown     TCPState = 0
	TCPStateEstablished TCPState = 1
	TCPStateSynSent     TCPState = 2
	TCPStateSynRecv     TCPState = 3
	TCPStateFinWait1    TCPState = 4
	TCPStateFinWait2    TCPState = 5
	TCPStateTimeWait    TCPState = 6
	TCPStateClose       TCPState = 7
	TCPStateCloseWait   TCPState = 8
	TCPStateLastAck     TCPState = 9
	TCPStateListen      TCPState = 10
	TCPStateClosing     TCPState = 11
	TCPStateClosed      TCPState = 12
)

var tcpStateDisplayNames = map[TCPState]string{
	TCPStateUnknown:     "UNKNOWN",
	TCPStateEstablished: "ESTABLISHED",
	TCPStateSynSent:     "SYN_SENT",
	TCPStateSynRecv:     "SYN_RECV",
	TCPStateFinWait1:    "FIN_WAIT1",
	TCPStateFinWait2:    "FIN_WAIT2",
	TCPStateTimeWait:    "TIME_WAIT",
	TCPStateClose:       "CLOSE",
	TCPStateCloseWait:   "CLOSE_WAIT",
	TCPStateLastAck:     "LAST_ACK",
	TCPStateListen:      "LISTEN",
	TCPStateClosing:     "CLOSING",
	TCPStateClosed:      "CLOSED",
}

func tcpStateFromLinux(state uint8) TCPState {
	switch state {
	case 1:
		return TCPStateEstablished
	case 2:
		return TCPStateSynSent
	case 3:
		return TCPStateSynRecv
	case 4:
		return TCPStateFinWait1
	case 5:
		return TCPStateFinWait2
	case 6:
		return TCPStateTimeWait
	case 7:
		return TCPStateClose
	case 8:
		return TCPStateCloseWait
	case 9:
		return TCPStateLastAck
	case 10:
		return TCPStateListen
	case 11:
		return TCPStateClosing
	default:
		return TCPStateUnknown
	}
}

func (s TCPState) String() string {
	if name, ok := tcpStateDisplayNames[s]; ok {
		return name
	}
	return fmt.Sprintf("STATE_%d", s)
}

func (s TCPState) IsTerminal() bool {
	switch s {
	case TCPStateClose, TCPStateClosed, TCPStateTimeWait:
		return true
	default:
		return false
	}
}

func (s TCPState) IsEstablished() bool {
	return s == TCPStateEstablished
}

type tcpConnectionState struct {
	SrcIP       string
	DstIP       string
	SrcPort     uint32
	DstPort     uint32
	State       TCPState
	LastUpdate  time.Time
	PID         uint32
	Comm        string
}

type tcpStateTracker struct {
	mu          sync.RWMutex
	connections map[string]*tcpConnectionState
}

func newTCPStateTracker() *tcpStateTracker {
	return &tcpStateTracker{
		connections: make(map[string]*tcpConnectionState),
	}
}

func (t *tcpStateTracker) connKey(srcIP, dstIP string, srcPort, dstPort uint32) string {
	return fmt.Sprintf("%s:%d->%s:%d", srcIP, srcPort, dstIP, dstPort)
}

func (t *tcpStateTracker) RecordStateChange(srcIP, dstIP string, srcPort, dstPort uint32, oldState, newState uint8, pid uint32, comm string) {
	if t == nil {
		return
	}
	key := t.connKey(srcIP, dstIP, srcPort, dstPort)
	newTCPState := tcpStateFromLinux(newState)

	t.mu.Lock()
	defer t.mu.Unlock()

	conn, ok := t.connections[key]
	if !ok {
		conn = &tcpConnectionState{
			SrcIP:   srcIP,
			DstIP:   dstIP,
			SrcPort: srcPort,
			DstPort: dstPort,
			PID:     pid,
			Comm:    comm,
		}
		t.connections[key] = conn
	}
	conn.State = newTCPState
	conn.LastUpdate = time.Now().UTC()
	if pid > 0 {
		conn.PID = pid
	}
	if comm != "" {
		conn.Comm = comm
	}
}

func (t *tcpStateTracker) RecordConnect(srcIP, dstIP string, srcPort, dstPort uint32, pid uint32, comm string) {
	if t == nil {
		return
	}
	key := t.connKey(srcIP, dstIP, srcPort, dstPort)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connections[key] = &tcpConnectionState{
		SrcIP:      srcIP,
		DstIP:      dstIP,
		SrcPort:    srcPort,
		DstPort:    dstPort,
		State:      TCPStateSynSent,
		LastUpdate: time.Now().UTC(),
		PID:        pid,
		Comm:       comm,
	}
}

func (t *tcpStateTracker) RecordClose(srcIP, dstIP string, srcPort, dstPort uint32) {
	if t == nil {
		return
	}
	key := t.connKey(srcIP, dstIP, srcPort, dstPort)
	t.mu.Lock()
	defer t.mu.Unlock()
	if conn, ok := t.connections[key]; ok {
		conn.State = TCPStateClosed
		conn.LastUpdate = time.Now().UTC()
	}
}

func (t *tcpStateTracker) Snapshot() []tcpConnectionState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	conns := make([]tcpConnectionState, 0, len(t.connections))
	for _, conn := range t.connections {
		conns = append(conns, *conn)
	}
	return conns
}

func (t *tcpStateTracker) EvictTerminalOlderThan(maxAge time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().UTC().Add(-maxAge)
	for key, conn := range t.connections {
		if conn.State.IsTerminal() && conn.LastUpdate.Before(cutoff) {
			delete(t.connections, key)
		}
	}
}

var tcpTracker = newTCPStateTracker()

func startTCPStateTrackerGC() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			tcpTracker.EvictTerminalOlderThan(1 * time.Minute)
		}
	}()
}

// ── Application Protocol Hints ────────────────────────────────────────

func detectAppProtocol(port uint32, domain string) string {
	p := uint16(port)
	switch p {
	case 80:
		return "HTTP"
	case 443:
		if strings.Contains(strings.ToLower(domain), "quic") {
			return "QUIC"
		}
		return "HTTPS/TLS"
	case 22:
		return "SSH"
	case 53:
		return "DNS"
	case 123:
		return "NTP"
	case 161, 162:
		return "SNMP"
	case 1883:
		return "MQTT"
	case 3306:
		return "MySQL"
	case 5432:
		return "PostgreSQL"
	case 6379:
		return "Redis"
	case 27017:
		return "MongoDB"
	case 9092:
		return "Kafka"
	case 6443:
		return "Kubernetes"
	default:
		if service := lookupService(p); service != "" {
			return service
		}
		return "Unknown"
	}
}

// ── Collectors integration ────────────────────────────────────────────

func recordNetworkFlowFromEvent(srcIP, dstIP string, srcPort, dstPort uint32, comm string, pid uint32, direction, state string) {
	protocol := "TCP"
	networkFlowAggregator.RecordConnection(srcIP, dstIP, srcPort, dstPort, protocol, comm, pid, direction, state)
}

func enrichEndpointWithContext(endpoint string) string {
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}

	// DNS enrichment
	if domain, ok := dnsCorrelation.LookupIP(host); ok {
		if portStr != "" {
			return net.JoinHostPort(domain, portStr)
		}
		return domain
	}

	return endpoint
}

func classifyEndpointScope(endpoint string) IPScope {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}
	ip := net.ParseIP(host)
	return classifyIPScope(ip)
}

// Parse a network endpoint string into IP scope, service, domain, and risk info
func analyzeEndpoint(endpoint string) (scope IPScope, service string, domain string, risk float64) {
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}

	// Scope classification
	ip := net.ParseIP(host)
	scope = classifyIPScope(ip)
	risk = ipScopeRiskScore(scope)

	// DNS enrichment
	if d, ok := dnsCorrelation.LookupIP(host); ok {
		domain = d
	}

	// Service name
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			service = lookupService(uint16(p))
			if isSuspiciousPortService(service) {
				risk = maxFloat64(risk, 0.80)
			}
		}
	}

	return
}
