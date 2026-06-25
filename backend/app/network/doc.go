// Package network — reserved for network event processing extraction.
//
// Target files (from app/): audit_network, dns_network, flow, geoip,
// scope_network, syscalls_network, tcp_network, tracker_bandwidth
//
// Prerequisites:
//   - Move protoDetectionEntry to a shared subpackage
//   - Move maxFloat64 to app/platform
//   - Resolve app-package type references
package network
