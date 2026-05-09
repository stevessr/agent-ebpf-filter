package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ── GeoIP enrichment (from rustnet geoip.rs) ──────────────────────────

type geoipRecord struct {
	Country   string `json:"country"`
	CountryCode string `json:"countryCode"`
	ASN       uint32 `json:"asn,omitempty"`
	ASNOrg    string `json:"asnOrg,omitempty"`
	City      string `json:"city,omitempty"`
}

type geoipCacheEntry struct {
	Record    geoipRecord
	ResolvedAt time.Time
}

type geoipResolver struct {
	mu       sync.RWMutex
	cache    map[string]*geoipCacheEntry // IP -> record
	maxCache int
	ttl      time.Duration
	hits     uint64
	misses   uint64
}

func newGeoipResolver() *geoipResolver {
	return &geoipResolver{
		cache:    make(map[string]*geoipCacheEntry),
		maxCache: 10000,
		ttl:      1 * time.Hour,
	}
}

var geoipDB = newGeoipResolver()

func (r *geoipResolver) Lookup(ipStr string) (geoipRecord, bool) {
	if r == nil {
		return geoipRecord{}, false
	}

	// Normalize
	ipStr = strings.TrimSpace(ipStr)
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return geoipRecord{}, false
	}

	// Cache lookup
	r.mu.RLock()
	entry, ok := r.cache[ipStr]
	r.mu.RUnlock()
	if ok && time.Since(entry.ResolvedAt) < r.ttl {
		return entry.Record, true
	}

	// Resolve
	record := r.resolveIP(ip)

	// Cache
	r.mu.Lock()
	if len(r.cache) >= r.maxCache {
		r.evictOldest()
	}
	r.cache[ipStr] = &geoipCacheEntry{
		Record:    record,
		ResolvedAt: time.Now().UTC(),
	}
	r.mu.Unlock()

	return record, true
}

func (r *geoipResolver) resolveIP(ip net.IP) geoipRecord {
	// Try MaxMind GeoLite2 database files
	if record, ok := lookupMaxMind(ip); ok {
		return record
	}

	// Fallback: IP range classification-based geo hints
	return classifyIPToRegion(ip)
}

func (r *geoipResolver) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	for key, entry := range r.cache {
		if oldestKey == "" || entry.ResolvedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.ResolvedAt
		}
	}
	if oldestKey != "" {
		delete(r.cache, oldestKey)
	}
}

func (r *geoipResolver) Stats() (hits, misses uint64) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hits, r.misses
}

// ── MaxMind GeoLite2 database lookup ──────────────────────────────────

var maxmindSearchPaths = []string{
	"./resources/geoip2",
	"./geoip",
	os.ExpandEnv("$XDG_DATA_HOME/rustnet/geoip"),
	"~/.local/share/rustnet/geoip",
	"/usr/share/GeoIP",
	"/usr/local/share/GeoIP",
	"/opt/homebrew/share/GeoIP",
	"/var/lib/GeoIP",
}

type maxmindDBHandle struct {
	path    string
	loaded  bool
	initErr error
}

var maxmindCountryDB = &maxmindDBHandle{}
var maxmindASNDB = &maxmindDBHandle{}
var maxmindCityDB = &maxmindDBHandle{}
var maxmindInitOnce sync.Once

func initMaxMindDatabases() {
	maxmindInitOnce.Do(func() {
		for _, basePath := range maxmindSearchPaths {
			expanded := expandPath(basePath)
			countryPath := filepath.Join(expanded, "GeoLite2-Country.mmdb")
			if _, err := os.Stat(countryPath); err == nil {
				maxmindCountryDB.path = countryPath
				maxmindCountryDB.loaded = true
			}
			asnPath := filepath.Join(expanded, "GeoLite2-ASN.mmdb")
			if _, err := os.Stat(asnPath); err == nil {
				maxmindASNDB.path = asnPath
				maxmindASNDB.loaded = true
			}
			cityPath := filepath.Join(expanded, "GeoLite2-City.mmdb")
			if _, err := os.Stat(cityPath); err == nil {
				maxmindCityDB.path = cityPath
				maxmindCityDB.loaded = true
			}
			if maxmindCountryDB.loaded || maxmindASNDB.loaded {
				log.Printf("[GEOIP] MaxMind databases found at %s", expanded)
				break
			}
		}
	})
}

func lookupMaxMind(ip net.IP) (geoipRecord, bool) {
	initMaxMindDatabases()

	if !maxmindCountryDB.loaded {
		return geoipRecord{}, false
	}

	// MaxMind lookup would use the maxminddb Go library.
	// For now, record that the database is available.
	// In production, integrate: github.com/oschwald/maxminddb-golang
	_ = ip
	return geoipRecord{}, false
}

// ── IP range-based region classification (fallback) ────────────────────

type ipRangeEntry struct {
	CIDR        string
	Country     string
	CountryCode string
	Org         string
}

var knownIPRanges = []ipRangeEntry{
	// China
	{"1.0.0.0/14", "China", "CN", "APNIC"},
	{"1.24.0.0/13", "China", "CN", "China Unicom"},
	{"14.0.0.0/8", "China", "CN", "China Telecom"},
	{"27.0.0.0/8", "China", "CN", "CNNIC"},
	{"36.0.0.0/8", "China", "CN", "CNNIC"},
	{"42.0.0.0/8", "China", "CN", "CNNIC"},
	{"49.0.0.0/8", "China", "CN", "CNNIC"},
	{"58.0.0.0/8", "China", "CN", "China Unicom"},
	{"59.0.0.0/8", "China", "CN", "China Telecom"},
	{"60.0.0.0/8", "China", "CN", "China Unicom"},
	{"61.0.0.0/8", "China", "CN", "CNNIC"},
	{"101.0.0.0/8", "China", "CN", "CNNIC"},
	{"110.0.0.0/8", "China", "CN", "China Telecom"},
	{"111.0.0.0/8", "China", "CN", "CNNIC"},
	{"112.0.0.0/8", "China", "CN", "CNNIC"},
	{"113.0.0.0/8", "China", "CN", "China Telecom"},
	{"114.0.0.0/8", "China", "CN", "China Unicom"},
	{"115.0.0.0/8", "China", "CN", "China Unicom"},
	{"116.0.0.0/8", "China", "CN", "China Telecom"},
	{"117.0.0.0/8", "China", "CN", "China Mobile"},
	{"118.0.0.0/8", "China", "CN", "China Telecom"},
	{"119.0.0.0/8", "China", "CN", "China Unicom"},
	{"120.0.0.0/8", "China", "CN", "China Unicom"},
	{"121.0.0.0/8", "China", "CN", "China Telecom"},
	{"122.0.0.0/8", "China", "CN", "CNNIC"},
	{"123.0.0.0/8", "China", "CN", "China Unicom"},
	{"124.0.0.0/8", "China", "CN", "China Telecom"},
	{"125.0.0.0/8", "China", "CN", "CNNIC"},
	{"171.0.0.0/8", "China", "CN", "CNNIC"},
	{"175.0.0.0/8", "China", "CN", "CNNIC"},
	{"180.0.0.0/8", "China", "CN", "CNNIC"},
	{"182.0.0.0/8", "China", "CN", "CNNIC"},
	{"183.0.0.0/8", "China", "CN", "CNNIC"},
	{"202.0.0.0/8", "China", "CN", "CNNIC"},
	{"203.0.0.0/8", "China", "CN", "APNIC"},
	{"210.0.0.0/8", "China", "CN", "APNIC"},
	{"211.0.0.0/8", "China", "CN", "APNIC"},
	{"218.0.0.0/8", "China", "CN", "CNNIC"},
	{"219.0.0.0/8", "China", "CN", "CNNIC"},
	{"220.0.0.0/8", "China", "CN", "CNNIC"},
	{"221.0.0.0/8", "China", "CN", "CNNIC"},
	{"222.0.0.0/8", "China", "CN", "China Telecom"},
	{"223.0.0.0/8", "China", "CN", "CNNIC"},

	// Russia
	{"2.60.0.0/14", "Russia", "RU", "Rostelecom"},
	{"5.0.0.0/12", "Russia", "RU", "Rostelecom"},
	{"31.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"37.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"46.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"62.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"77.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"78.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"79.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"80.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"82.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"83.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"84.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"85.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"87.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"88.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"89.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"90.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"91.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"92.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"93.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"94.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"95.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"109.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"128.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"130.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"145.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"146.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"176.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"178.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"185.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"188.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"194.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"195.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"212.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"213.0.0.0/8", "Russia", "RU", "Rostelecom"},
	{"217.0.0.0/8", "Russia", "RU", "Rostelecom"},

	// North Korea
	{"175.45.176.0/22", "North Korea", "KP", "Star JV"},

	// Iran
	{"2.144.0.0/12", "Iran", "IR", "DCI"},
	{"5.22.0.0/16", "Iran", "IR", "DCI"},
	{"5.52.0.0/16", "Iran", "IR", "DCI"},
	{"5.56.128.0/17", "Iran", "IR", "DCI"},
	{"5.74.0.0/15", "Iran", "IR", "DCI"},
	{"5.160.0.0/12", "Iran", "IR", "DCI"},
	{"31.0.0.0/8", "Iran", "IR", "DCI"},
	{"37.0.0.0/8", "Iran", "IR", "DCI"},
	{"46.0.0.0/8", "Iran", "IR", "DCI"},
	{"78.0.0.0/8", "Iran", "IR", "DCI"},
	{"80.0.0.0/8", "Iran", "IR", "DCI"},
	{"81.0.0.0/8", "Iran", "IR", "DCI"},
	{"82.0.0.0/8", "Iran", "IR", "DCI"},
	{"83.0.0.0/8", "Iran", "IR", "DCI"},
	{"84.0.0.0/8", "Iran", "IR", "DCI"},
	{"85.0.0.0/8", "Iran", "IR", "DCI"},
	{"86.0.0.0/8", "Iran", "IR", "DCI"},
	{"87.0.0.0/8", "Iran", "IR", "DCI"},
	{"88.0.0.0/8", "Iran", "IR", "DCI"},
	{"89.0.0.0/8", "Iran", "IR", "DCI"},
	{"90.0.0.0/8", "Iran", "IR", "DCI"},
	{"91.0.0.0/8", "Iran", "IR", "DCI"},
	{"92.0.0.0/8", "Iran", "IR", "DCI"},
	{"93.0.0.0/8", "Iran", "IR", "DCI"},
	{"94.0.0.0/8", "Iran", "IR", "DCI"},
	{"95.0.0.0/8", "Iran", "IR", "DCI"},

	// US (major cloud/tech)
	{"3.0.0.0/8", "United States", "US", "AWS"},
	{"4.0.0.0/8", "United States", "US", "Level 3"},
	{"8.0.0.0/8", "United States", "US", "Level 3"},
	{"13.0.0.0/8", "United States", "US", "AWS"},
	{"15.0.0.0/8", "United States", "US", "AWS"},
	{"17.0.0.0/8", "United States", "US", "Apple"},
	{"18.0.0.0/8", "United States", "US", "MIT"},
	{"20.0.0.0/8", "United States", "US", "Microsoft"},
	{"23.0.0.0/8", "United States", "US", "Akamai"},
	{"34.0.0.0/8", "United States", "US", "Google Cloud"},
	{"35.0.0.0/8", "United States", "US", "Google Cloud"},
	{"40.0.0.0/8", "United States", "US", "Microsoft"},
	{"44.0.0.0/8", "United States", "US", "Amateur Radio"},
	{"45.0.0.0/8", "United States", "US", "Various"},
	{"47.0.0.0/8", "United States", "US", "Bell Canada"},
	{"50.0.0.0/8", "United States", "US", "Comcast"},
	{"52.0.0.0/8", "United States", "US", "AWS"},
	{"54.0.0.0/8", "United States", "US", "AWS"},
	{"63.0.0.0/8", "United States", "US", "Verizon"},
	{"64.0.0.0/8", "United States", "US", "Verizon"},
	{"65.0.0.0/8", "United States", "US", "Comcast"},
	{"66.0.0.0/8", "United States", "US", "Comcast"},
	{"67.0.0.0/8", "United States", "US", "Comcast"},
	{"68.0.0.0/8", "United States", "US", "Comcast"},
	{"69.0.0.0/8", "United States", "US", "Comcast"},
	{"70.0.0.0/8", "United States", "US", "Comcast"},
	{"71.0.0.0/8", "United States", "US", "Comcast"},
	{"72.0.0.0/8", "United States", "US", "Various"},
	{"73.0.0.0/8", "United States", "US", "Comcast"},
	{"74.0.0.0/8", "United States", "US", "Comcast"},
	{"75.0.0.0/8", "United States", "US", "Comcast"},
	{"76.0.0.0/8", "United States", "US", "Comcast"},
	{"96.0.0.0/8", "United States", "US", "Comcast"},
	{"97.0.0.0/8", "United States", "US", "Comcast"},
	{"98.0.0.0/8", "United States", "US", "Comcast"},
	{"104.0.0.0/8", "United States", "US", "Cloudflare"},
	{"107.0.0.0/8", "United States", "US", "Various"},
	{"108.0.0.0/8", "United States", "US", "Comcast"},
	{"136.0.0.0/8", "United States", "US", "Various"},
	{"142.0.0.0/8", "United States", "US", "Various"},
	{"152.0.0.0/8", "United States", "US", "Various"},
	{"157.0.0.0/8", "United States", "US", "Microsoft"},
	{"172.0.0.0/8", "United States", "US", "Various"},

	// EU
	{"2.0.0.0/8", "France", "FR", "Orange"},
	{"38.0.0.0/8", "United Kingdom", "GB", "Various"},
	{"41.0.0.0/8", "South Africa", "ZA", "AfriNIC"},
	{"43.0.0.0/8", "Australia", "AU", "APNIC"},
	{"51.0.0.0/8", "United Kingdom", "GB", "Various"},
	{"53.0.0.0/8", "Germany", "DE", "Deutsche Telekom"},
	{"57.0.0.0/8", "France", "FR", "Orange"},
	{"81.0.0.0/8", "United Kingdom", "GB", "Various"},
	{"133.0.0.0/8", "Japan", "JP", "JPNIC"},
	{"139.0.0.0/8", "United Kingdom", "GB", "Various"},
	{"141.0.0.0/8", "Germany", "DE", "Various"},
	{"144.0.0.0/8", "Australia", "AU", "APNIC"},
	{"150.0.0.0/8", "Japan", "JP", "JPNIC"},
	{"151.0.0.0/8", "Italy", "IT", "Various"},
	{"153.0.0.0/8", "Japan", "JP", "JPNIC"},
	{"155.0.0.0/8", "United Kingdom", "GB", "Various"},
	{"156.0.0.0/8", "South Africa", "ZA", "AfriNIC"},
	{"159.0.0.0/8", "Germany", "DE", "Various"},
	{"160.0.0.0/8", "South Africa", "ZA", "AfriNIC"},
	{"161.0.0.0/8", "United Kingdom", "GB", "Various"},
	{"162.0.0.0/8", "United States", "US", "Various"},
	{"163.0.0.0/8", "Japan", "JP", "JPNIC"},
	{"164.0.0.0/8", "Various", "XX", "Various"},
	{"165.0.0.0/8", "Various", "XX", "Various"},
	{"166.0.0.0/8", "Various", "XX", "Various"},
	{"167.0.0.0/8", "Various", "XX", "Various"},
	{"168.0.0.0/8", "Various", "XX", "Various"},
	{"169.0.0.0/8", "Various", "XX", "Various"},
	{"170.0.0.0/8", "Various", "XX", "Various"},
	{"173.0.0.0/8", "United States", "US", "Comcast"},
	{"174.0.0.0/8", "United States", "US", "Comcast"},
	{"184.0.0.0/8", "Canada", "CA", "Bell Canada"},
	{"192.0.0.0/8", "Various", "XX", "Various"},
	{"193.0.0.0/8", "Belgium", "BE", "RIPE"},
	{"196.0.0.0/8", "South Africa", "ZA", "AfriNIC"},
	{"197.0.0.0/8", "South Africa", "ZA", "AfriNIC"},
	{"198.0.0.0/8", "Various", "XX", "Various"},
	{"199.0.0.0/8", "United States", "US", "ARIN"},
	{"200.0.0.0/8", "Brazil", "BR", "LACNIC"},
	{"201.0.0.0/8", "Brazil", "BR", "LACNIC"},
	{"204.0.0.0/8", "Various", "XX", "Various"},
	{"205.0.0.0/8", "Various", "XX", "Various"},
	{"206.0.0.0/8", "United States", "US", "ARIN"},
	{"207.0.0.0/8", "United States", "US", "ARIN"},
	{"208.0.0.0/8", "United States", "US", "ARIN"},
	{"209.0.0.0/8", "United States", "US", "ARIN"},
	{"214.0.0.0/8", "United States", "US", "DoD"},
	{"215.0.0.0/8", "United States", "US", "DoD"},
	{"216.0.0.0/8", "United States", "US", "ARIN"},
}

var knownIPNets []*net.IPNet

var geoipInitOnce sync.Once

func initGeoIPDatabase() {
	geoipInitOnce.Do(func() {
		knownIPNets = make([]*net.IPNet, 0, len(knownIPRanges))
		for _, entry := range knownIPRanges {
			_, ipnet, err := net.ParseCIDR(entry.CIDR)
			if err != nil {
				continue
			}
			knownIPNets = append(knownIPNets, ipnet)
		}
		log.Printf("[GEOIP] loaded %d IP range entries", len(knownIPNets))
	})
}

func classifyIPToRegion(ip net.IP) geoipRecord {
	initGeoIPDatabase()

	for i, ipnet := range knownIPNets {
		if ipnet.Contains(ip) {
			entry := knownIPRanges[i]
			return geoipRecord{
				Country:     entry.Country,
				CountryCode: entry.CountryCode,
				ASNOrg:      entry.Org,
			}
		}
	}

	// Default: unknown
	return geoipRecord{
		Country:     "Unknown",
		CountryCode: "XX",
	}
}

func isHighRiskCountry(countryCode string) bool {
	highRisk := map[string]bool{
		"KP": true, // North Korea
		"IR": true, // Iran
		"SY": true, // Syria
		"CU": true, // Cuba
		"SD": true, // Sudan
	}
	return highRisk[strings.ToUpper(countryCode)]
}

// ── Helper ───────────────────────────────────────────────────────────

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// Enrich an endpoint with GeoIP information.
func enrichEndpointWithGeoIP(endpoint string) string {
	host := endpoint
	if h, _, err := net.SplitHostPort(endpoint); err == nil {
		host = h
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return endpoint
	}

	// Skip private/local IPs
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return endpoint
	}

	record, ok := geoipDB.Lookup(host)
	if !ok || record.CountryCode == "XX" {
		return endpoint
	}

	if record.CountryCode != "" {
		return fmt.Sprintf("%s [%s/%s]", endpoint, record.Country, record.CountryCode)
	}
	return endpoint
}
