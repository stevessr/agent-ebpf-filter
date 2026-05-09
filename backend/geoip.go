package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
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

// ── Pure-Go MaxMind MMDB reader ────────────────────────────────────────

const (
	mmdbMetadataStartMarker = "\xAB\xCD\xEFMaxMind.com"
)

type mmdbReader struct {
	data       []byte
	metadata   mmdbMetadata
	dataStart  int // offset in file where data section begins
}

type mmdbMetadata struct {
	NodeCount       uint32            `mmdb:"node_count"`
	RecordSize      uint16            `mmdb:"record_size"`
	IPVersion       uint16            `mmdb:"ip_version"`
	DatabaseType    string            `mmdb:"database_type"`
	Languages       []string          `mmdb:"languages"`
	Description     map[string]string `mmdb:"description"`
	BinaryFormatMajor uint16          `mmdb:"binary_format_major_version"`
	BuildEpoch      uint64            `mmdb:"build_epoch"`
}

var maxmindCountryDB = &mmdbReader{}
var maxmindASNDB = &mmdbReader{}
var maxmindCityDB = &mmdbReader{}
var maxmindInitOnce sync.Once

func initMaxMindDatabases() {
	maxmindInitOnce.Do(func() {
		for _, basePath := range maxmindSearchPaths {
			expanded := expandPath(basePath)
			countryPath := filepath.Join(expanded, "GeoLite2-Country.mmdb")
			if db, err := openMMDB(countryPath); err == nil {
				*maxmindCountryDB = *db
				log.Printf("[GEOIP] loaded Country DB: %s", countryPath)
			}
			asnPath := filepath.Join(expanded, "GeoLite2-ASN.mmdb")
			if db, err := openMMDB(asnPath); err == nil {
				*maxmindASNDB = *db
				log.Printf("[GEOIP] loaded ASN DB: %s", asnPath)
			}
			cityPath := filepath.Join(expanded, "GeoLite2-City.mmdb")
			if db, err := openMMDB(cityPath); err == nil {
				*maxmindCityDB = *db
				log.Printf("[GEOIP] loaded City DB: %s", cityPath)
			}
			if maxmindCountryDB.data != nil || maxmindASNDB.data != nil {
				log.Printf("[GEOIP] MaxMind databases found at %s", expanded)
				break
			}
		}
	})
}

func openMMDB(path string) (*mmdbReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if len(data) < 20 {
		return nil, fmt.Errorf("mmdb: file too small")
	}

	marker := mmdbMetadataStartMarker
	markerOffset := len(data) - len(marker)
	if string(data[markerOffset:]) != marker {
		return nil, fmt.Errorf("mmdb: invalid marker")
	}

	// Read metadata pointer (last 4 bytes before marker, big-endian)
	metaPtr := int(binary.BigEndian.Uint32(data[markerOffset-4:]))
	if metaPtr >= len(data) {
		return nil, fmt.Errorf("mmdb: metadata pointer out of range")
	}

	decoder := &mmdbDecoder{data: data, offset: metaPtr}
	metaRaw, err := decoder.decode()
	if err != nil {
		return nil, fmt.Errorf("mmdb: failed to decode metadata: %w", err)
	}
	metaMap, ok := metaRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("mmdb: metadata is not a map")
	}

	meta := mmdbMetadata{}
	if v, ok := metaMap["node_count"]; ok {
		meta.NodeCount = toUint32(v)
	}
	if v, ok := metaMap["record_size"]; ok {
		meta.RecordSize = uint16(toUint32(v))
	}
	if v, ok := metaMap["ip_version"]; ok {
		meta.IPVersion = uint16(toUint32(v))
	}
	if v, ok := metaMap["database_type"]; ok {
		meta.DatabaseType = toString(v)
	}
	if v, ok := metaMap["binary_format_major_version"]; ok {
		meta.BinaryFormatMajor = uint16(toUint32(v))
	}
	if v, ok := metaMap["build_epoch"]; ok {
		meta.BuildEpoch = toUint64(v)
	}
	meta.Languages = toStringSlice(metaMap["languages"])
	if desc, ok := metaMap["description"]; ok {
		if descMap, ok := desc.(map[string]interface{}); ok {
			meta.Description = make(map[string]string, len(descMap))
			for k, v := range descMap {
				meta.Description[k] = toString(v)
			}
		}
	}

	if meta.NodeCount == 0 || meta.RecordSize == 0 {
		return nil, fmt.Errorf("mmdb: invalid metadata")
	}

	nodeSize := int(meta.RecordSize) * 2 / 8
	dataStart := int(meta.NodeCount) * nodeSize + 16 // +16 for padding

	return &mmdbReader{
		data:      data,
		metadata:  meta,
		dataStart: dataStart,
	}, nil
}

func (r *mmdbReader) lookup(ip net.IP) (interface{}, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("mmdb: not loaded")
	}

	var ipBytes []byte
	if ip4 := ip.To4(); ip4 != nil {
		ipBytes = ip4
	} else if ip16 := ip.To16(); ip16 != nil {
		ipBytes = ip16
	} else {
		return nil, fmt.Errorf("mmdb: invalid IP")
	}

	recordSize := int(r.metadata.RecordSize)
	nodeSize := recordSize * 2 / 8
	nodeCount := int(r.metadata.NodeCount)

	// Binary search through the tree
	nodeNum := 0
	for depth := 0; depth < len(ipBytes)*8; depth++ {
		if nodeNum >= nodeCount {
			break
		}
		bit := (ipBytes[depth/8] >> (7 - uint(depth%8))) & 1
		offset := nodeNum*nodeSize + int(bit)*recordSize/8
		if offset >= r.dataStart {
			break
		}
		nodeNum = int(r.readNodeRecord(offset))
	}

	// nodeNum is now the data pointer (>= nodeCount means data)
	if nodeNum < nodeCount {
		return nil, fmt.Errorf("mmdb: search ended at node pointer")
	}
	dataOffset := r.dataStart + (nodeNum - nodeCount)
	if dataOffset >= len(r.data) {
		return nil, fmt.Errorf("mmdb: data offset out of range")
	}

	decoder := &mmdbDecoder{data: r.data, offset: dataOffset}
	return decoder.decode()
}

func (r *mmdbReader) readNodeRecord(offset int) uint32 {
	recordSize := int(r.metadata.RecordSize)
	switch recordSize {
	case 24:
		return uint32(r.data[offset])<<16 | uint32(r.data[offset+1])<<8 | uint32(r.data[offset+2])
	case 28:
		return uint32(r.data[offset])<<20 | uint32(r.data[offset+1])<<12 | uint32(r.data[offset+2])<<4 | uint32(r.data[offset+3])>>4
	case 32:
		return binary.BigEndian.Uint32(r.data[offset : offset+4])
	default:
		return 0
	}
}

// ── MMDB Data Decoder (MessagePack-like with MMDB extensions) ─────────

type mmdbDecoder struct {
	data   []byte
	offset int
}

func (d *mmdbDecoder) decode() (interface{}, error) {
	if d.offset >= len(d.data) {
		return nil, fmt.Errorf("mmdb decode: offset out of range")
	}
	ctrl := d.data[d.offset]
	d.offset++

	switch {
	case ctrl <= 0xbf:
		// Reserved range (0x00-0xbf) — includes pointers
		// Values 0x00-0x1f: may be a pointer depending on context
		// But in MMDB data, values in metadata use standard type encoding
		return d.decodeExtended(ctrl)
	case ctrl >= 0xc0 && ctrl <= 0xdf:
		return d.decodeExtended(ctrl)
	case ctrl >= 0xe0:
		return int(int8(ctrl)), nil
	default:
		return nil, fmt.Errorf("mmdb decode: unknown control byte 0x%02x", ctrl)
	}
}

func (d *mmdbDecoder) decodeExtended(ctrl byte) (interface{}, error) {
	switch {
	case ctrl == 0xc0:
		return nil, nil
	case ctrl == 0xc1:
		// never used, but reserved
		return nil, fmt.Errorf("mmdb: reserved 0xc1")
	case ctrl == 0xc2:
		return false, nil
	case ctrl == 0xc3:
		return true, nil
	case ctrl == 0xc4:
		return d.readBytes(1)
	case ctrl == 0xc5:
		return d.readBytes(2)
	case ctrl == 0xc6:
		return d.readBytes(4)
	case ctrl == 0xc7:
		return d.readBytes(8)
	case ctrl == 0xc8:
		return d.readUint(1)
	case ctrl == 0xc9:
		return d.readUint(2)
	case ctrl == 0xca:
		return d.readFloat32()
	case ctrl == 0xcb:
		return d.readFloat64()
	case ctrl == 0xcc:
		return d.readUint8()
	case ctrl == 0xcd:
		return d.readUint16()
	case ctrl == 0xce:
		return d.readUint32()
	case ctrl == 0xcf:
		return d.readUint64()
	case ctrl == 0xd0:
		return d.readInt8()
	case ctrl == 0xd1:
		return d.readInt16()
	case ctrl == 0xd2:
		return d.readInt32()
	case ctrl == 0xd3:
		return d.readInt64()
	case ctrl == 0xd4:
		return d.readFixExt(1)
	case ctrl == 0xd5:
		return d.readFixExt(2)
	case ctrl == 0xd6:
		return d.readFixExt(4)
	case ctrl == 0xd7:
		return d.readFixExt(8)
	case ctrl == 0xd8:
		return d.readFixExt(16)
	case ctrl == 0xd9:
		return d.readStr(1)
	case ctrl == 0xda:
		return d.readStr(2)
	case ctrl == 0xdb:
		return d.readStr(4)
	case ctrl == 0xdc:
		return d.readArray(2)
	case ctrl == 0xdd:
		return d.readArray(4)
	case ctrl == 0xde:
		return d.readMap(2)
	case ctrl == 0xdf:
		return d.readMap(4)
	default:
		if ctrl <= 0x7f {
			return int(ctrl), nil
		}
		if ctrl >= 0x80 && ctrl <= 0x8f {
			return d.readMap(int(ctrl & 0x0f))
		}
		if ctrl >= 0x90 && ctrl <= 0x9f {
			return d.readArray(int(ctrl & 0x0f))
		}
		if ctrl >= 0xa0 && ctrl <= 0xbf {
			return d.readStr(int(ctrl & 0x1f))
		}
		return nil, fmt.Errorf("mmdb: unhandled control 0x%02x", ctrl)
	}
}

func (d *mmdbDecoder) readBytes(sizeLen int) ([]byte, error) {
	size := d.readUintRaw(sizeLen)
	if d.offset+int(size) > len(d.data) {
		return nil, fmt.Errorf("mmdb: bytes out of range")
	}
	result := make([]byte, size)
	copy(result, d.data[d.offset:d.offset+int(size)])
	d.offset += int(size)
	return result, nil
}

func (d *mmdbDecoder) readStr(sizeLen int) (string, error) {
	raw, err := d.readBytes(sizeLen)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (d *mmdbDecoder) readArray(sizeLen int) ([]interface{}, error) {
	count := int(d.readUintRaw(sizeLen))
	result := make([]interface{}, count)
	for i := 0; i < count; i++ {
		v, err := d.decode()
		if err != nil {
			return nil, err
		}
		result[i] = v
	}
	return result, nil
}

func (d *mmdbDecoder) readMap(sizeLen int) (map[string]interface{}, error) {
	count := int(d.readUintRaw(sizeLen))
	result := make(map[string]interface{}, count)
	for i := 0; i < count; i++ {
		keyVal, err := d.decode()
		if err != nil {
			return nil, err
		}
		key, ok := keyVal.(string)
		if !ok {
			key = toString(keyVal)
		}
		val, err := d.decode()
		if err != nil {
			return nil, err
		}
		result[key] = val
	}
	return result, nil
}

func (d *mmdbDecoder) readUint(sizeLen int) (uint64, error) {
	return d.readUintRaw(sizeLen), nil
}

func (d *mmdbDecoder) readUintRaw(sizeLen int) uint64 {
	if d.offset+sizeLen > len(d.data) {
		return 0
	}
	var v uint64
	for i := 0; i < sizeLen; i++ {
		v = (v << 8) | uint64(d.data[d.offset+i])
	}
	d.offset += sizeLen
	return v
}

func (d *mmdbDecoder) readUint8() (uint8, error) {
	if d.offset >= len(d.data) {
		return 0, fmt.Errorf("mmdb: eof at uint8")
	}
	v := d.data[d.offset]
	d.offset++
	return v, nil
}

func (d *mmdbDecoder) readUint16() (uint16, error) {
	if d.offset+2 > len(d.data) {
		return 0, fmt.Errorf("mmdb: eof at uint16")
	}
	v := binary.BigEndian.Uint16(d.data[d.offset:])
	d.offset += 2
	return v, nil
}

func (d *mmdbDecoder) readUint32() (uint32, error) {
	if d.offset+4 > len(d.data) {
		return 0, fmt.Errorf("mmdb: eof at uint32")
	}
	v := binary.BigEndian.Uint32(d.data[d.offset:])
	d.offset += 4
	return v, nil
}

func (d *mmdbDecoder) readUint64() (uint64, error) {
	if d.offset+8 > len(d.data) {
		return 0, fmt.Errorf("mmdb: eof at uint64")
	}
	v := binary.BigEndian.Uint64(d.data[d.offset:])
	d.offset += 8
	return v, nil
}

func (d *mmdbDecoder) readInt8() (int8, error) {
	v, err := d.readUint8()
	return int8(v), err
}

func (d *mmdbDecoder) readInt16() (int16, error) {
	v, err := d.readUint16()
	return int16(v), err
}

func (d *mmdbDecoder) readInt32() (int32, error) {
	v, err := d.readUint32()
	return int32(v), err
}

func (d *mmdbDecoder) readInt64() (int64, error) {
	v, err := d.readUint64()
	return int64(v), err
}

func (d *mmdbDecoder) readFloat32() (float32, error) {
	v, err := d.readUint32()
	return float32frombits(v), err
}

func (d *mmdbDecoder) readFloat64() (float64, error) {
	v, err := d.readUint64()
	return float64frombits(v), err
}

func float32frombits(b uint32) float32 {
	return math.Float32frombits(b)
}

func float64frombits(b uint64) float64 {
	return math.Float64frombits(b)
}

func (d *mmdbDecoder) readFixExt(size int) ([]byte, error) {
	if d.offset+size > len(d.data) {
		return nil, fmt.Errorf("mmdb: fixext out of range")
	}
	result := make([]byte, size)
	copy(result, d.data[d.offset:d.offset+size])
	d.offset += size
	return result, nil
}

func toUint32(v interface{}) uint32 {
	switch val := v.(type) {
	case uint32:
		return val
	case float64:
		return uint32(val)
	case uint64:
		return uint32(val)
	case int:
		return uint32(val)
	case int64:
		return uint32(val)
	}
	return 0
}

func toUint64(v interface{}) uint64 {
	switch val := v.(type) {
	case uint64:
		return val
	case float64:
		return uint64(val)
	case uint32:
		return uint64(val)
	case int:
		return uint64(val)
	}
	return 0
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}

func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		result = append(result, toString(item))
	}
	return result
}

func lookupMaxMind(ip net.IP) (geoipRecord, bool) {
	initMaxMindDatabases()

	if maxmindCountryDB.data == nil {
		return geoipRecord{}, false
	}

	raw, err := maxmindCountryDB.lookup(ip)
	if err != nil || raw == nil {
		return geoipRecord{}, false
	}

	record := geoipRecord{}

	if countryMap, ok := raw.(map[string]interface{}); ok {
		if country, ok := countryMap["country"]; ok {
			if cm, ok := country.(map[string]interface{}); ok {
				if names, ok := cm["names"]; ok {
					if nm, ok := names.(map[string]interface{}); ok {
						if en, ok := nm["en"]; ok {
							record.Country = toString(en)
						}
					}
				}
				if iso, ok := cm["iso_code"]; ok {
					record.CountryCode = toString(iso)
				}
			}
		}
	}

	// Try ASN database
	if maxmindASNDB.data != nil {
		if asnRaw, err := maxmindASNDB.lookup(ip); err == nil && asnRaw != nil {
			if asnMap, ok := asnRaw.(map[string]interface{}); ok {
				if asn, ok := asnMap["autonomous_system_number"]; ok {
					record.ASN = toUint32(asn)
				}
				if org, ok := asnMap["autonomous_system_organization"]; ok {
					record.ASNOrg = toString(org)
				}
			}
		}
	}

	// Try City database for city name
	if maxmindCityDB.data != nil {
		if cityRaw, err := maxmindCityDB.lookup(ip); err == nil && cityRaw != nil {
			if cityMap, ok := cityRaw.(map[string]interface{}); ok {
				if city, ok := cityMap["city"]; ok {
					if cm, ok := city.(map[string]interface{}); ok {
						if names, ok := cm["names"]; ok {
							if nm, ok := names.(map[string]interface{}); ok {
								if en, ok := nm["en"]; ok {
									record.City = toString(en)
								}
							}
						}
					}
				}
				// City DB may also have country info (more precise)
				if record.Country == "" {
					if country, ok := cityMap["country"]; ok {
						if cm, ok := country.(map[string]interface{}); ok {
							if names, ok := cm["names"]; ok {
								if nm, ok := names.(map[string]interface{}); ok {
									if en, ok := nm["en"]; ok {
										record.Country = toString(en)
									}
								}
							}
							if record.CountryCode == "" {
								if iso, ok := cm["iso_code"]; ok {
									record.CountryCode = toString(iso)
								}
							}
						}
					}
				}
				// Subdivisions
				if subs, ok := cityMap["subdivisions"]; ok {
					if subArr, ok := subs.([]interface{}); ok && len(subArr) > 0 {
						if sub, ok := subArr[0].(map[string]interface{}); ok {
							if names, ok := sub["names"]; ok {
								if nm, ok := names.(map[string]interface{}); ok {
									if en, ok := nm["en"]; ok && record.City == "" {
										record.City = toString(en)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if record.Country == "" && record.CountryCode == "" {
		return geoipRecord{}, false
	}

	return record, true
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
