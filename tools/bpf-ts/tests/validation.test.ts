import { describe, expect, test } from "bun:test";
import { compileBpfTs } from "../src/compiler";

describe("bpf-ts verifier-aware validation", () => {
  test("requires explicit i32 probe return types", () => {
    expect(() => compileBpfTs(`
class P {
  @kprobe("x")
  static p(ctx: KProbeContext) { return 0; }
}
`, "missing-return-type.ts")).toThrow("explicit ': i32'");

    expect(() => compileBpfTs(`
class P {
  @kprobe("x")
  static p(ctx: KProbeContext): u64 { return 0; }
}
`, "wrong-return-type.ts")).toThrow("must return i32");
  });

  test("requires explicit probe context types", () => {
    expect(() => compileBpfTs(`
class P {
  @kprobe("x")
  static p(ctx): i32 { return 0; }
}
`, "missing-context-type.ts")).toThrow("explicit context type");
  });

  test("requires complete ringbuf payloads", () => {
    expect(() => compileBpfTs(`
interface Event { pid: u32; uid: u32; }
const events = ringbuf<Event>(65536);
class P {
  @kprobe("x")
  static p(ctx: KProbeContext): i32 {
    events.emit({ pid: bpf.pid() });
    return 0;
  }
}
`, "missing-field.ts")).toThrow("missing required field: uid");
  });

  test("rejects fields not present in the ringbuf struct", () => {
    expect(() => compileBpfTs(`
interface Event { pid: u32; }
const events = ringbuf<Event>(65536);
class P {
  @kprobe("x")
  static p(ctx: KProbeContext): i32 {
    events.emit({ pid: bpf.pid(), uid: bpf.uid() });
    return 0;
  }
}
`, "extra-field.ts")).toThrow("has no field 'uid'");
  });
});
