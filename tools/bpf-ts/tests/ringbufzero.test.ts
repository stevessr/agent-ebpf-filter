import { describe, expect, test } from "bun:test";
import { compileBpfTs } from "../src/compiler";

const largeEvent = `
interface Event {
  timestampNs: u64;
  length: i32;
  reserved: u32;
  sample: bytes<4096>;
}
const events = ringbuf<Event>(1 << 20);
class P {
  @uprobe("SSL_write")
  static capture(ctx: UProbeContext): i32 {
    const buffer = bpf.arg(ctx, 2);
    const length = bpf.argI32(ctx, 3);
    if (length > 0) {
      events.emit({
        timestampNs: bpf.ktimeNs(),
        length,
        reserved: 0,
        sample: bpf.userBytes(buffer, length),
      });
    }
    return 0;
  }
}
`;

describe("large ringbuf zeroing", () => {
  test("lowers large record initialization without a surviving memset call", () => {
    const result = compileBpfTs(largeEvent, "large-ringbuf.ts");
    expect(result.cSource).not.toContain("__builtin_memset(__bpf_ts_event_");
    expect(result.cSource).toContain("#pragma clang loop unroll(disable)");
    expect(result.cSource).toContain("sizeof(__u64) <= sizeof(*__bpf_ts_event_");
    expect(result.cSource).toContain("*(__u64 *)((unsigned char *)__bpf_ts_event_");
    expect(result.cSource).toContain("((unsigned char *)__bpf_ts_event_");
    expect(result.cSource).toContain("bpf_probe_read_user");
  });

  test("keeps the compact memset path for small ringbuf values", () => {
    const result = compileBpfTs(`
interface Event { pid: u32; sample: bytes<64>; }
const events = ringbuf<Event>(65536);
class P {
  @uprobe("SSL_write")
  static capture(ctx: UProbeContext): i32 {
    const buffer = bpf.arg(ctx, 2);
    events.emit({ pid: bpf.pid(), sample: bpf.userBytes(buffer, 64) });
    return 0;
  }
}
`, "small-ringbuf.ts");
    expect(result.cSource).toContain("__builtin_memset(__bpf_ts_event_");
    expect(result.cSource).not.toContain("_zero_off");
  });
});
