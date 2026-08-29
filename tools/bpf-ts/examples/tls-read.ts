/// <reference path="../dsl.d.ts" />

interface TLSReadEvent {
  pid: u32;
  tid: u32;
  length: i32;
  sample: bytes<64>;
}

// One outstanding SSL_read buffer per thread. takeOr copies and removes the
// pending entry before the return value is interpreted, so EOF/errors cannot
// leave stale state for the next SSL_read on this thread.
const pendingReadBuffers = hash<u32, u64>(16384);
const tlsReads = ringbuf<TLSReadEvent>(1 << 20);

export class TLSReadProbes {
  @uprobe("SSL_read")
  static enter(ctx: UProbeContext): i32 {
    const tid = bpf.tid();
    const buffer = bpf.arg(ctx, 2);
    pendingReadBuffers.set(tid, buffer);
    return 0;
  }

  @uretprobe("SSL_read")
  static complete(ctx: UProbeContext): i32 {
    const tid = bpf.tid();
    const buffer = pendingReadBuffers.takeOr(tid, 0);
    const length = bpf.retI32(ctx);
    if (buffer !== 0 && length > 0) {
      tlsReads.emit({
        pid: bpf.pid(),
        tid,
        length,
        sample: bpf.userBytes(buffer, length),
      });
    }
    return 0;
  }
}
