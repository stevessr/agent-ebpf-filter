package app

import (
	"agent-ebpf-filter/internal/geoip"
)

// ---- moved from backend/zz_merged_backend.go section geoip.go ----

type geoipRecord = geoip.Record
type geoipResolver = geoip.Resolver

var geoipDB = geoip.NewResolver()

func newGeoipResolver() *geoipResolver {
	return geoip.NewResolver()
}

func initGeoIPDatabase() {
	AppCtx.Network.InitGeoIPDatabase()
}

func isHighRiskCountry(countryCode string) bool {
	return geoip.IsHighRiskCountry(countryCode)
}

func enrichEndpointWithGeoIP(endpoint string) string {
	return AppCtx.Network.EnrichEndpointWithGeoIP(endpoint)
}
