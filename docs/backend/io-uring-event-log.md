# Experimental io_uring runtime event log backend

The eBPF data plane still uses **BPF ring buffers**. io_uring is a separate,
user-space file-I/O mechanism; it does not replace eBPF ringbuf or policy
enforcement. This optional backend applies only to the Go runtime JSONL event
log writer (`backend/app/runtime_event_log_writer.go`), not model inference,
WebSocket transport, recording exports or the eBPF probes.

## Configure

By default the runtime uses the existing `bufio.Writer -> os.File.Write` path.

```sh
# Explicitly request the experimental backend before starting the backend:
export AGENT_EVENT_LOG_IO=io_uring

# Remove or set to any other value for the original buffered writer:
unset AGENT_EVENT_LOG_IO
```

The backend reports `ioBackend` as either `io_uring` or `buffered-write`
inside its runtime event log status. If the kernel, seccomp profile, container
or file descriptor rejects io_uring setup, the backend logs one warning and
**falls back before writing a single record**. No syscall auto-retry with
`write(2)` occurs after an io_uring request is submitted: the request could
already have partially completed, so retrying could duplicate audit entries.

Only Linux amd64 and arm64 currently have the raw-syscall implementation;
other platforms build against the safe stub. The writer requires an
`O_APPEND` regular file and `IORING_FEAT_RW_CUR_POS` support.

## Ordering and failure semantics

* A single in-flight SQE preserves the append order of the existing JSONL
  audit hash chain. A flush barrier succeeds only after the kernel reports
  completed writes, not merely successful submissions.
* A short write, CQE error or wait failure is terminal to the log generation.
  Existing failure and drop counters are preserved. Never blindly retry an
  ambiguous write.
* The mode retains the original queue capacity (4,096 records), 128-record
  batches, 256 KiB buffered writes and 250 ms periodic flush.
* Flush means data has reached the OS write completion boundary, **not** power
  loss durability. Neither backend adds an `fsync` per flush.
* The feature is *not* enabled automatically: io_uring can add overhead for
  cached buffered writes and enables additional kernel attack surface.

## Validate before enabling in production

```sh
cd backend
go test ./internal/uringwrite -v
go test ./app -run 'TestRuntimeEventLogWriter' -count=1
go test ./internal/uringwrite -run '^$' -bench 'BenchmarkAppend' -benchmem -count=5

# Compare event ingestion, p50/p95/p99 persistence latency, CPU and queue
# saturation under your representative workloads with the env var on/off.
```

A local container prototype on an AMD EPYC host showed a **regression** at
128 KiB per operation: roughly 29.6 microseconds for buffered `write(2)` versus
48.9 microseconds for a single in-flight io_uring `WRITE`. These numbers are
diagnostic, not representative of production. The feature is therefore
experimental and requires proof of end-to-end benefit before default enablement.

Future work: a bounded multi-SQE writer with strict append ordering, CQE
accounting and cancellation-safe flush barriers, plus sustained-load regression
testing. Simply increasing SQE concurrency without preserving audit order
would be unsafe.
