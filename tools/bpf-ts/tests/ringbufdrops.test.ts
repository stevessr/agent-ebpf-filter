import { describe, expect, test } from "bun:test";
import { compileBpfTs } from "../src/compiler";

function program(sampleSize: number) {
  return `
interface Event { pid: u32; sample: bytes<${sampleSize}>; }
const events = ringbuf<Event>(1 << 20);
class P {
  @uprobe("SSL_write")
  static write(ctx: UProbeContext): i32 {
    const buffer = bpf.arg(ctx, 2);
    const length = bpf.argI32(ctx, 3);
    if (length > 0) {
      events.emit({ pid: bpf.pid(), sample: bpf.userBytes(buffer, length) });
    }
    return 0;
  }
}
`;
}

describe("compact ringbuf loss accounting", () => {
  test("adds separate per-CPU output-drop and user-read counters", () => {
    const result = compileBpfTs(program(4096), "large.ts");
    expect(result.manifest.maps).toContainEqual({
      name: "__bpf_ts_drops_events",
      kind: "percpu_array",
      maxEntries: 1,
    });
    expect(result.manifest.maps).toContainEqual({
      name: "__bpf_ts_read_errors_events",
      kind: "percpu_array",
      maxEntries: 1,
    });
    expect(result.cSource).toContain("BPF_MAP_TYPE_PERCPU_ARRAY");
    expect(result.cSource).toContain("static __always_inline void __bpf_ts_note_drop_events(void)");
    expect(result.cSource).toContain("static __always_inline long __bpf_ts_output_events");
    expect(result.cSource).toContain("if (rc != 0) __bpf_ts_note_drop_events();");
    expect(result.cSource).toContain("static __always_inline void __bpf_ts_note_read_error_events(void)");
    const readCheck = result.cSource.indexOf("if (__bpf_ts_read_rc_");
    const readError = result.cSource.indexOf("__bpf_ts_note_read_error_events();", readCheck);
    const zeroLength = result.cSource.indexOf("} else {", readError);
    expect(readCheck).toBeGreaterThanOrEqual(0);
    expect(readError).toBeGreaterThan(readCheck);
    expect(zeroLength).toBeGreaterThan(readError);
    expect(result.cSource).toContain("__bpf_ts_output_events(");
  });

  test("leaves small reserve/submit ringbufs uninstrumented", () => {
    const result = compileBpfTs(program(64), "small.ts");
    expect(result.manifest.maps.some((map) => map.name === "__bpf_ts_drops_events")).toBe(false);
    expect(result.manifest.maps.some((map) => map.name === "__bpf_ts_read_errors_events")).toBe(false);
    expect(result.cSource).not.toContain("__bpf_ts_note_drop_events");
    expect(result.cSource).not.toContain("__bpf_ts_note_read_error_events");
    expect(result.cSource).toContain("bpf_ringbuf_reserve(&events");
    expect(result.cSource).toContain("bpf_ringbuf_submit(");
  });
});
