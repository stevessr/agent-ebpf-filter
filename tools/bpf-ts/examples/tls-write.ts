/// <reference path="../dsl.d.ts" />

interface TLSWriteEvent {
  pid: u32;
  length: u64;
  timestampNs: u64;
  sample: bytes<64>;
}

const tlsWrites = ringbuf<TLSWriteEvent>(1 << 20);

export class TLSProbes {
  @uprobe("SSL_write")
  static sslWrite(ctx: UProbeContext): i32 {
    const buffer = bpf.arg(ctx, 2);
    const length = bpf.arg(ctx, 3);
    tlsWrites.emit({
      pid: bpf.pid(),
      length,
      timestampNs: bpf.ktimeNs(),
      sample: bpf.userBytes(buffer, length),
    });
    return 0;
  }
}
