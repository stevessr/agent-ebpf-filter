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

describe("large ringbuf lowering", () => {
  test("uses per-cpu scratch and variable-length output for a large trailing userBytes field", () => {
    const result = compileBpfTs(largeEvent, "large-ringbuf.ts");
    expect(result.cSource).toContain("BPF_MAP_TYPE_PERCPU_ARRAY");
    expect(result.cSource).toContain("__bpf_ts_scratch_events");
    expect(result.cSource).toContain("bpf_ringbuf_output(&events");
    expect(result.cSource).toContain("__builtin_offsetof(struct Event, sample)");
    expect(result.cSource).not.toContain("bpf_ringbuf_reserve(&events");
    expect(result.cSource).not.toContain("_zero_off");
    expect(result.manifest.maps).toContainEqual({
      name: "__bpf_ts_scratch_events",
      kind: "percpu_array",
      maxEntries: 1,
    });
  });

  test("keeps reserve+memset for small ringbuf values", () => {
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
    expect(result.cSource).toContain("bpf_ringbuf_reserve(&events");
    expect(result.cSource).toContain("__builtin_memset(__bpf_ts_event_");
    expect(result.cSource).not.toContain("__bpf_ts_scratch_events");
    expect(result.cSource).not.toContain("_zero_off");
  });

  test("retains bounded zeroing as fallback when a large byte field is not the trailing compact payload", () => {
    const result = compileBpfTs(`
interface Event { sample: bytes<4096>; pid: u32; }
const events = ringbuf<Event>(1 << 20);
class P {
  @uprobe("SSL_write")
  static capture(ctx: UProbeContext): i32 {
    const buffer = bpf.arg(ctx, 2);
    events.emit({ sample: bpf.userBytes(buffer, 64), pid: bpf.pid() });
    return 0;
  }
}
`, "large-fallback.ts");
    expect(result.cSource).toContain("bpf_ringbuf_reserve(&events");
    expect(result.cSource).toContain("#pragma clang loop unroll(disable)");
    expect(result.cSource).toContain("_zero_off");
    expect(result.cSource).not.toContain("__bpf_ts_scratch_events");
  });
});
