package geoip

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// ── Pure-Go MaxMind MMDB reader ────────────────────────────────────────

type mmdbReader struct {
	data      []byte
	metadata  mmdbMetadata
	dataStart int // offset in file where data section begins
}

type mmdbMetadata struct {
	NodeCount         uint32            `mmdb:"node_count"`
	RecordSize        uint16            `mmdb:"record_size"`
	IPVersion         uint16            `mmdb:"ip_version"`
	DatabaseType      string            `mmdb:"database_type"`
	Languages         []string          `mmdb:"languages"`
	Description       map[string]string `mmdb:"description"`
	BinaryFormatMajor uint16            `mmdb:"binary_format_major_version"`
	BuildEpoch        uint64            `mmdb:"build_epoch"`
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
	dataStart := int(meta.NodeCount)*nodeSize + 16 // +16 for padding

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

func lookupMaxMind(ip net.IP) (Record, bool) {
	initMaxMindDatabases()

	if maxmindCountryDB.data == nil {
		return Record{}, false
	}

	raw, err := maxmindCountryDB.lookup(ip)
	if err != nil || raw == nil {
		return Record{}, false
	}

	record := Record{}

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
		return Record{}, false
	}

	return record, true
}

// ── IP range-based region classification (fallback) ────────────────────
