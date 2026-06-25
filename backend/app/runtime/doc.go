// Package runtime provides the lifecycle management for the agent backend,
// including privilege elevation, eBPF bootstrap, process cleanup, port
// selection, and the global RuntimeSettings store.
//
// This package is under construction as part of the app/ package refactoring.
// Currently the implementation lives in app/ files prefixed with:
//   - runtime_ebpf.go   — eBPF map bootstrap and pin management
//   - statepersistenceruntime.go — RuntimeSettings persistence
//   - stateenvruntime.go        — env-var-based configuration
//   - statetypesruntime.go      — type aliases to core package
//   - startup_server.go        — port selection / .port file
//   - cleanup_process.go       — kill stale backend processes
//   - jobs_background.go       — periodic background jobs
//   - health_bootstrap.go      — startup health bootstrap
//   - c_runtime.go             — ML C inference runtime
package runtime
