package main

import "agent-ebpf-filter/internal/geoip"

type geoipRecord = geoip.Record
type geoipResolver = geoip.Resolver

var geoipDB = geoip.NewResolver()

func newGeoipResolver() *geoipResolver {
	return geoip.NewResolver()
}

func initGeoIPDatabase() {
	geoip.InitDatabase()
}

func isHighRiskCountry(countryCode string) bool {
	return geoip.IsHighRiskCountry(countryCode)
}

func enrichEndpointWithGeoIP(endpoint string) string {
	return geoip.EnrichEndpointWithGeoIP(geoipDB, endpoint)
}
