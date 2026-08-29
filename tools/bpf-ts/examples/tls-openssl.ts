/// <reference path="../dsl.d.ts" />

// Fixed userspace ABI consumed by backend/app/tls/bpfts_openssl.go.
// Layout is intentionally explicit: 48 bytes of metadata followed by a
// verifier-bounded 4096-byte plaintext preview, for a total of 4144 bytes.
interface OpenSSLEvent {
  timestampNs: u64;
  connectionId: u64;
  pid: u32;
  tid: u32;
  length: i32;
  direction: u8;
  function: u8;
  reserved: u16;
  comm: bytes<16>;
  sample: bytes<4096>;
}

const pendingReadBuffers = hash<u32, u64>(16384);
const pendingReadConnections = hash<u32, u64>(16384);
const tlsOpenSSLEvents = ringbuf<OpenSSLEvent>(1 << 20);

export class OpenSSLTLSProbes {
  @uprobe("SSL_write")
  static write(ctx: UProbeContext): i32 {
    const connectionId = bpf.arg(ctx, 1);
    const buffer = bpf.arg(ctx, 2);
    const length = bpf.argI32(ctx, 3);
    const tid = bpf.tid();
    if (buffer !== 0 && length > 0) {
      tlsOpenSSLEvents.emit({
        timestampNs: bpf.ktimeNs(),
        connectionId,
        pid: bpf.pid(),
        tid,
        length,
        direction: 1,
        function: 1,
        reserved: 0,
        comm: bpf.comm(),
        sample: bpf.userBytes(buffer, length),
      });
    }
    return 0;
  }

  @uprobe("SSL_read")
  static readEnter(ctx: UProbeContext): i32 {
    const tid = bpf.tid();
    const connectionId = bpf.arg(ctx, 1);
    const buffer = bpf.arg(ctx, 2);
    pendingReadConnections.set(tid, connectionId);
    pendingReadBuffers.set(tid, buffer);
    return 0;
  }

  @uretprobe("SSL_read")
  static readComplete(ctx: UProbeContext): i32 {
    const tid = bpf.tid();
    const connectionId = pendingReadConnections.takeOr(tid, 0);
    const buffer = pendingReadBuffers.takeOr(tid, 0);
    const length = bpf.retI32(ctx);
    if (buffer !== 0 && length > 0) {
      tlsOpenSSLEvents.emit({
        timestampNs: bpf.ktimeNs(),
        connectionId,
        pid: bpf.pid(),
        tid,
        length,
        direction: 0,
        function: 2,
        reserved: 0,
        comm: bpf.comm(),
        sample: bpf.userBytes(buffer, length),
      });
    }
    return 0;
  }
}
