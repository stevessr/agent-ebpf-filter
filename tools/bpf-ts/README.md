# bpf-ts

`bpf-ts` is an experimental, verifier-aware TypeScript-like DSL compiler for eBPF.
It deliberately supports a small, statically checkable subset of TypeScript rather
than attempting to run JavaScript semantics in the kernel.

## Pipeline

```text
TypeScript source
  -> TypeScript AST
  -> restricted BPF IR
  -> semantic / verifier-resource passes
  -> libbpf C
  -> clang -target bpf
  -> BPF ELF + attach manifest
  -> Go bpfts loader
```

The generated C is kept as a debugging artifact. The current CI compiles every
example for both x86_64 and arm64 BPF register ABIs, checks CO-RE BTF sections,
runs the Go loader tests, and verifies that every generated ELF program/map/section
matches its manifest exactly.

## Probe surface

Probe methods are decorated **static methods** in a namespace-style class:

```ts
class Probes {
  @kprobe("wake_up_new_task")
  static enter(ctx: KProbeContext): i32 { return 0; }

  @kretprobe("do_sys_open")
  static kernelReturn(ctx: KProbeContext): i32 {
    const ret = bpf.ret(ctx);
    return 0;
  }

  @uprobe("SSL_read")
  static sslReadEnter(ctx: UProbeContext): i32 { return 0; }

  @uretprobe("SSL_read")
  static sslReadReturn(ctx: UProbeContext): i32 {
    const ret = bpf.retI32(ctx);
    return 0;
  }
}
```

Supported attach kinds are `kprobe`, `kretprobe`, `uprobe`, `uretprobe`, and
`tracepoint`. `bpf.arg(ctx, N)` is entry-probe-only. `bpf.ret()` and
`bpf.retI32()` are return-probe-only.

`bpf.retI32()` is useful for APIs such as OpenSSL `SSL_read`, whose ABI return
value is a signed C `int`; it prevents an error return such as `-1` from being
mistaken for a large positive length.

## Entry -> return state

Scalar `hash` maps support `set`, `getOr`, `takeOr`, `delete`, and `increment`.
For entry/return correlation, prefer `takeOr`:

```ts
const pending = hash<u32, u64>(16384);

@uprobe("SSL_read")
static enter(ctx: UProbeContext): i32 {
  const tid = bpf.tid();
  pending.set(tid, bpf.arg(ctx, 2));
  return 0;
}

@uretprobe("SSL_read")
static exit(ctx: UProbeContext): i32 {
  const tid = bpf.tid();
  const buffer = pending.takeOr(tid, 0);
  const length = bpf.retI32(ctx);
  // ...
  return 0;
}
```

`takeOr` lowers to lookup -> copy value -> delete. Its key must be a local
identifier so lookup and delete are guaranteed to use the same evaluated value.
This also keeps the map-value pointer from escaping across deletion.

The current TLS example keys pending calls by TID. This is suitable for ordinary
non-reentrant calls but is **not** a call-depth stack; nested/reentrant calls to
the same function on one thread can overwrite the pending value. A composite
key/call-depth map is a future extension.

## Safe user reads

`bpf.userBytes(pointer, length)` does not blindly fill the destination field.
Generated C clamps the requested length to the fixed `bytes<N>` destination
before calling `bpf_probe_read_user`.

## CO-RE

Kernel BTF projections use `declare interface` so they are distinct from event
payload structs:

```ts
declare interface task_struct {
  pid: i32;
  tgid: i32;
}

const pid = bpf.coreRead.task_struct.pid(taskPtr);
```

This lowers to `BPF_CORE_READ` and preserves the declared scalar width and
signedness. Kernel projection interfaces cannot be used directly as map/event
value structs. The initial CO-RE backend supports scalar fields only.

## Verifier-aware restrictions

The compiler rejects unsafe or unsupported constructs before C generation,
including unbounded loops, excessive unrolling, recursion, `any`, async/generator
functions, unsupported dynamic property access, invalid helper contexts, large
local byte arrays, shadowing of protected identifiers, malformed map operations,
and ambiguous CO-RE access.

This is still a compiler policy layer, not a replacement for the kernel eBPF
verifier.

## Build / smoke

From `tools/bpf-ts`:

```bash
bun install
bun run build
bun test
bun src/cli.ts examples/tls-read.ts \
  --out generated/tls-read.bpf.c \
  --manifest generated/tls-read.json
```

The repository workflow `.github/workflows/bpf-ts-smoke.yml` additionally runs
real clang BPF compilation for x86_64 and arm64 and validates the produced ELF
with `backend/app/bpfts`.

## Runtime boundary

`backend/app/bpfts` provides strict manifest parsing, exact ELF/manifest
validation, kprobe/kretprobe/tracepoint attachment, and uprobe/uretprobe
attachment through an injected resolver. The resolver can return a symbol or an
explicit `Address`/`Offset`/`PID`, allowing integration with stripped-binary
resolution without duplicating that logic in the compiler.

GitHub-hosted smoke CI is intentionally unprivileged: successful CI proves that
the compiler, clang BPF backend, BTF metadata and userspace loader contracts are
consistent. It does **not** prove that every generated object will load and attach
on every target kernel. Privileged target-kernel verifier/attach testing remains
a separate deployment check.
