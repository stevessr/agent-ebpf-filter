// Package tls — reserved for TLS capture subsystem extraction.
//
// Target files (from app/): agentstreamtls.go, agentstreamlooptls.go,
// capturecontrollertls.go, capturehandlerstls.go, capturerulestls.go,
// capturestartuptls.go, capturestoretls.go, capturetypestls.go,
// fragmentassemblertls.go, httpparsertls.go, httpstreamassemblertls.go,
// probediscoveryrustls.go, probediscoverytls.go, probemanager_builtin.go,
// probemanagerrustls.go, probemanagertls.go, capturesinkcodex.go,
// keyremoval.go, ai_enrichment.go, analyzers_agentsight.go
//
// Prerequisites:
//   - Resolve processContext dependency on context_event.go
//   - Resolve TLSProbeManager dependency on probemanagertls.go
//   - Export lowercase types (tlsFragment, completedTLSFragment)
//   - Move app-level TLS type consumers (handlers_agentsight.go)
package tls
