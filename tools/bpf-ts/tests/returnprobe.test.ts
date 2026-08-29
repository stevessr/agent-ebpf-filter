import { describe, expect, test } from "bun:test";
import { compileBpfTs } from "../src/compiler";

const sslReadProgram = `
interface Event { tid: u32; len: i32; sample: bytes<32>; }
const pending = hash<u32, u64>(1024);
const events = ringbuf<Event>(65536);
class TLS {
  @uprobe("SSL_read")
  static enter(ctx: UProbeContext): i32 {
    const tid = bpf.tid();
    pending.set(tid, bpf.arg(ctx, 2));
    return 0;
  }

  @uretprobe("SSL_read")
  static exit(ctx: UProbeContext): i32 {
    const tid = bpf.tid();
    const buffer = pending.getOr(tid, 0);
    pending.delete(tid);
    const len = bpf.retI32(ctx);
    if (buffer !== 0 && len > 0) {
      events.emit({ tid, len, sample: bpf.userBytes(buffer, len) });
    }
    return 0;
  }
}
`;

describe("bpf-ts return probes", () => {
  test("lowers uretprobe, return value, map lookup and cleanup", () => {
    const result = compileBpfTs(sslReadProgram, "tls-read.ts");
    expect(result.manifest.probes).toContainEqual({
      name: "exit",
      kind: "uretprobe",
      section: "uretprobe",
      target: "SSL_read",
    });
    expect(result.cSource).toContain('SEC("uretprobe")');
    expect(result.cSource).toContain("PT_REGS_RC(ctx)");
    expect(result.cSource).toContain("((__s32)PT_REGS_RC(ctx))");
    expect(result.cSource).toContain("bpf_map_lookup_elem(&pending");
    expect(result.cSource).toContain("bpf_map_delete_elem(&pending");
    expect(result.cSource).toContain("? *");
  });

  test("lowers kretprobe sections and native-width return values", () => {
    const result = compileBpfTs(`
interface Event { ret: i64; }
const events = ringbuf<Event>(65536);
class P {
  @kretprobe("do_sys_open")
  static exit(ctx: KProbeContext): i32 {
    const ret = bpf.ret(ctx);
    events.emit({ ret });
    return 0;
  }
}
`, "kret.ts");
    expect(result.manifest.probes[0]).toEqual({
      name: "exit",
      kind: "kretprobe",
      section: "kretprobe/do_sys_open",
      target: "do_sys_open",
    });
    expect(result.cSource).toContain('SEC("kretprobe/do_sys_open")');
    expect(result.cSource).toContain("((__s64)PT_REGS_RC(ctx))");
  });

  test("rejects argument reads from return probes", () => {
    expect(() => compileBpfTs(`
class P {
  @uretprobe("SSL_read")
  static exit(ctx: UProbeContext): i32 {
    const value = bpf.arg(ctx, 1);
    return 0;
  }
}
`, "bad-return-arg.ts")).toThrow("bpf.arg() is not valid in uretprobe");
  });

  test("rejects return helpers from entry probes", () => {
    expect(() => compileBpfTs(`
class P {
  @uprobe("SSL_read")
  static enter(ctx: UProbeContext): i32 {
    const value = bpf.retI32(ctx);
    return 0;
  }
}
`, "bad-entry-ret.ts")).toThrow("only valid in kretprobe/uretprobe");
  });

  test("requires getOr to be a direct local initializer", () => {
    expect(() => compileBpfTs(`
const pending = hash<u32, u64>(1024);
class P {
  @uprobe("x")
  static enter(ctx: UProbeContext): i32 {
    if (pending.getOr(bpf.tid(), 0) !== 0) { return 0; }
    return 0;
  }
}
`, "nested-getor.ts")).toThrow("must be used as a direct local initializer");
  });

  test("rejects delete on array maps", () => {
    expect(() => compileBpfTs(`
const values = array<u64>(16);
class P {
  @kprobe("x")
  static enter(ctx: KProbeContext): i32 {
    values.delete(0);
    return 0;
  }
}
`, "array-delete.ts")).toThrow("not valid for array map");
  });
});
