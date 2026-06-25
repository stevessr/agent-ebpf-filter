package network

import (
	"agent-ebpf-filter/internal/geoip"
)

// ---- moved from backend/zz_merged_backend.go section geoip.go ----

type geoipRecord = geoip.Record
type geoipResolver = geoip.Resolver

func newGeoipResolver() *geoipResolver {
	return geoip.NewResolver()
}

func initGeoIPDatabase() {
	geoip.InitDatabase()
}

func isHighRiskCountry(countryCode string) bool {
	return geoip.IsHighRiskCountry(countryCode)
}

func enrichEndpointWithGeoIP(resolver *geoipResolver, endpoint string) string {
	return geoip.EnrichEndpointWithGeoIP(resolver, endpoint)
}
