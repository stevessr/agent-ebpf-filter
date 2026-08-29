/// <reference path="../dsl.d.ts" />

interface TLSReadEvent {
  pid: u32;
  tid: u32;
  length: i32;
  sample: bytes<64>;
}

// One outstanding SSL_read buffer per thread. The return probe always deletes
// the entry before evaluating the captured return value so stale state cannot
// leak into a later call on the same thread.
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
  static exit(ctx: UProbeContext): i32 {
    const tid = bpf.tid();
    const buffer = pendingReadBuffers.getOr(tid, 0);
    pendingReadBuffers.delete(tid);
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
