# Runtime audit-log write-path experiments

This document covers the Linux JSONL runtime event log in `backend/app/runtime_event_log_writer.go`.
It does **not** replace the eBPF ring buffer, change enforcement decisions, or affect
the separate Pebble/recording backends.

## Recommendation before tuning

Keep the existing single-goroutine `bufio.Writer` (256 KiB, 128-event batches,
250 ms timer) as the production default until application-level benchmarks
demonstrate a faster alternative. The existing implementation already issues
**one bulk write** for typical batches: replacing it with a single-request
io_uring WRITE adds ring submission/completion overhead.

The actual audit write path includes protobuf envelope normalization, a
SHA-256 chained digest, JSON marshaling, a bounded Go channel, the file sink,
and optional readers. Its throughput cannot be inferred from file-write
microbenchmarks alone.

## Changes in this experiment

1. **One normalization pass**: `AuditChain.MarshalRecord` now passes the
   already-normalized record to the digest helper. The verification path still
   normalizes untrusted replay records before recomputing the hash.
2. **Optional `writev` batches**: `AGENT_EVENT_LOG_IO=writev` uses an
   `O_APPEND` regular file, retains at most 128 serialized JSONL lines and
   emits them with one vectored syscall. It avoids the extra copy from each
   marshaled line into the 256 KiB `bufio` area, but retains those allocations
   until flush and incurs iovec setup overhead.
3. **Fallback and guardrails**: unsupported platforms and descriptors fall
   back before any I/O. A returned short write, syscall error, or terminal
   failure is never blindly retried: an append may already be partially
   committed. Queue limits, flush barriers, redaction and audit schema are
   unchanged.

Configure:

```sh
# Default buffered path
unset AGENT_EVENT_LOG_IO

# Explicit opt-in on Linux with regular O_APPEND event log
export AGENT_EVENT_LOG_IO=writev
```

The runtime event-log status reports `ioBackend` as `buffered-write` or
`writev`. A completed write or flush is **not** an `fsync` durability
guarantee against power loss.

## What to measure

Use an identical event stream (small/medium/large records) and identical
storage for A/B runs. Collect events per second, end-to-end p50/p95/p99 ingest
latency, pending/dropped/failed records, CPU usage, heap allocations and
GC pauses. Ensure the final JSONL stream has the same number of events and
valid `recording.VerifyAuditChain` results. Compare low-load as well as
queue-saturated workloads.

```sh
cd backend
go test ./internal/eventlogio -run '^Test' -count=1
go test ./app/recording -run '^TestAuditChain' -count=1
go test ./app -run '^TestRuntimeEventLog' -count=1
go test ./internal/eventlogio -run '^$' -bench '^Benchmark(Buffered|Writev)' -benchmem -count=5
go test ./app/recording -run '^$' -bench '^BenchmarkAuditHashNormalization' -benchmem -count=5
```

## Alternatives intentionally not enabled

- **Multiple writers sharing one JSONL file**: file-append interleaving and
  audit sequence ordering become more difficult; speedup is not automatic.
- **`O_DIRECT`**: alignment and partial-write rules complicate short JSONL
  records. Page cache is usually appropriate for a live log.
- **`mmap` append**: requires explicit file growth and synchronization and
  makes partial-record recovery more complex.
- **`fallocate`**: may help sustained large file growth when used with
  `FALLOC_FL_KEEP_SIZE`, but primarily reserves disk blocks rather than
  removing serialization overhead. Requires disk-space policy and filesystem
  compatibility tests.
- **`fdatasync` per record**: improves durability expectations but typically
  hurts throughput; should be a separate, explicit durability policy.

This remains an experiment, not a claimed throughput improvement.
