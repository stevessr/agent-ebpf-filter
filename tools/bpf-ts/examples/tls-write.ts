/// <reference path="../dsl.d.ts" />

interface TLSWriteEvent {
  pid: u32;
  length: u64;
  timestampNs: u64;
}

const tlsWrites = ringbuf<TLSWriteEvent>(1 << 20);

@uprobe("SSL_write")
export function sslWrite(ctx: UProbeContext): i32 {
  const length = bpf.arg(ctx, 3);
  tlsWrites.emit({
    pid: bpf.pid(),
    length,
    timestampNs: bpf.ktimeNs(),
  });
  return 0;
}
