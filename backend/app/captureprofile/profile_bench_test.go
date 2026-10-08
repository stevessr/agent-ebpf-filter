package captureprofile

import (
	"fmt"
	"testing"
)

func benchmarkRegistry(size int) *Registry {
	profiles := make([]Profile, 0, size)
	protocols := []string{"http1", "http2", "grpc"}
	methods := []string{"GET", "POST", "PUT", "PATCH"}
	sources := []string{"kernel_socket_prefix", "tls_plaintext"}
	for i := 0; i < size; i++ {
		profiles = append(profiles, Profile{
			ID: fmt.Sprintf("bench.%04d", i), Vendor: "bench",
			Sources: []string{sources[i%len(sources)]}, Protocols: []string{protocols[i%len(protocols)]},
			Directions: []string{"outgoing"}, Methods: []string{methods[i%len(methods)]},
			Transports: []string{"tcp"}, HostSuffixes: []string{fmt.Sprintf("api-%d.example.test", i)},
			PathPrefixes: []string{"/v1"}, MinScore: 90,
		})
	}
	profiles = append(profiles, Profile{
		ID: "target", Vendor: "target", Sources: []string{"kernel_socket_prefix"}, Protocols: []string{"http1"},
		Directions: []string{"outgoing"}, Methods: []string{"POST"}, Transports: []string{"tcp"},
		HostSuffixes: []string{"target.example.test"}, PathPrefixes: []string{"/v1/jobs"}, MinScore: 90,
	})
	return NewRegistry(profiles)
}

func BenchmarkRegistryMatchIndexed512(b *testing.B) {
	registry := benchmarkRegistry(512)
	observation := Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Transport: "tcp", Host: "target.example.test", Path: "/v1/jobs/42"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if match, ok := registry.Match(observation); !ok || match.ProfileID != "target" {
			b.Fatal("target profile did not match")
		}
	}
}

func linearMatchForBenchmark(registry *Registry, observation Observation) (Match, bool) {
	snapshot := registry.snapshot.Load()
	observation = normalizeObservation(observation)
	best := Match{}
	matched := false
	for _, profile := range snapshot.profiles {
		candidate, ok := matchProfile(profile, observation)
		if !ok {
			continue
		}
		if !matched || candidate.Score > best.Score || (candidate.Score == best.Score && candidate.ProfileID < best.ProfileID) {
			best = candidate
			matched = true
		}
	}
	return best, matched
}

func BenchmarkRegistryMatchLinear512(b *testing.B) {
	registry := benchmarkRegistry(512)
	observation := Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Transport: "tcp", Host: "target.example.test", Path: "/v1/jobs/42"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if match, ok := linearMatchForBenchmark(registry, observation); !ok || match.ProfileID != "target" {
			b.Fatal("target profile did not match")
		}
	}
}

func BenchmarkRegistryMatchCompact512(b *testing.B) {
	registry := benchmarkRegistry(512)
	observation := Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Transport: "tcp", Host: "target.example.test", Path: "/v1/jobs/42"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if match, ok := registry.MatchCompact(observation); !ok || match.ProfileID != "target" {
			b.Fatal("target profile did not match")
		}
	}
}
