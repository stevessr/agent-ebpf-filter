package network

import (
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// ── DNS → IP Correlation Cache (rustnet reverse-DNS concept) ─────────

type dnsEntry struct {
	Domain     string
	IP         string
	ResolvedAt time.Time
	TTL        time.Duration
}

type DNSCache struct {
	mu       sync.RWMutex
	entries  map[string]*dnsEntry // IP -> domain mapping
	byDomain map[string]string    // domain -> IP reverse mapping
}

type DNSCacheSnapshotEntry struct {
	Domain     string `json:"domain"`
	IP         string `json:"ip"`
	ResolvedAt int64  `json:"resolvedAt"`
	ExpiresAt  int64  `json:"expiresAt"`
	TTLSeconds int64  `json:"ttlSeconds"`
}

func NewDNSCache() *DNSCache {
	return &DNSCache{
		entries:  make(map[string]*dnsEntry),
		byDomain: make(map[string]string),
	}
}

func (c *DNSCache) Record(domain, ip string) {
	c.RecordWithTTL(domain, ip, 5*time.Minute)
}

func (c *DNSCache) RecordWithTTL(domain, ip string, ttl time.Duration) {
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
		TTL:        ttl,
	}
	c.entries[ip] = entry
	c.byDomain[domain] = ip
}

func (c *DNSCache) LookupIP(ip string) (string, bool) {
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

func (c *DNSCache) LookupDomain(domain string) (string, bool) {
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

func (c *DNSCache) EnrichEndpoint(endpoint string) string {
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

func (c *DNSCache) EvictExpired() {
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

func (c *DNSCache) Snapshot() []DNSCacheSnapshotEntry {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := time.Now()
	entries := make([]DNSCacheSnapshotEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		if entry == nil || entry.IP == "" || entry.Domain == "" {
			continue
		}
		expiresAt := entry.ResolvedAt.Add(entry.TTL)
		if now.After(expiresAt) {
			continue
		}
		entries = append(entries, DNSCacheSnapshotEntry{
			Domain:     entry.Domain,
			IP:         entry.IP,
			ResolvedAt: entry.ResolvedAt.UnixMilli(),
			ExpiresAt:  expiresAt.UnixMilli(),
			TTLSeconds: int64(time.Until(expiresAt).Seconds()),
		})
	}
	return entries
}

func CorrelateDNSResponse(cache *DNSCache, rawData []byte) {
	var parser dnsmessage.Parser
	header, err := parser.Start(rawData)
	if err != nil || !header.Response {
		return
	}
	questions := make([]string, 0, 2)
	for {
		question, err := parser.Question()
		if err == dnsmessage.ErrSectionDone {
			break
		}
		if err != nil {
			return
		}
		questions = append(questions, strings.TrimSuffix(question.Name.String(), "."))
	}
	for {
		answer, err := parser.Answer()
		if err == dnsmessage.ErrSectionDone {
			break
		}
		if err != nil {
			return
		}
		domain := strings.TrimSuffix(answer.Header.Name.String(), ".")
		if domain == "" && len(questions) > 0 {
			domain = questions[0]
		}
		ttl := time.Duration(answer.Header.TTL) * time.Second
		if ttl <= 0 {
			ttl = 5 * time.Minute
		}
		switch body := answer.Body.(type) {
		case *dnsmessage.AResource:
			cache.RecordWithTTL(domain, net.IP(body.A[:]).String(), ttl)
		case *dnsmessage.AAAAResource:
			cache.RecordWithTTL(domain, net.IP(body.AAAA[:]).String(), ttl)
		}
	}
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

func LookupService(port uint16) string {
	if name, ok := portToService[port]; ok {
		return name
	}
	return ""
}

func LookupServiceByPort(port uint32) string {
	return LookupService(uint16(port))
}

func IsSuspiciousPortService(serviceName string) bool {
	return suspiciousPortServices[serviceName]
}
