import { describe, expect, test } from "bun:test";
import { compileBpfTs } from "../src/compiler";
import { BpfTsCompileError } from "../src/diagnostics";

const tracepointProgram = `
interface Event {
  pid: u32;
  uid: u32;
  comm: bytes<16>;
}
const events = ringbuf<Event>(1 << 20);
const counts = hash<u32, u64>(1024);
class Probes {
  @tracepoint("syscalls", "sys_enter_execve")
  static onExec(ctx: TracepointContext): i32 {
    const pid = bpf.pid();
    counts.increment(pid);
    for (let i = 0; i < 4; i++) {
      counts.set(pid, i);
    }
    events.emit({ pid, uid: bpf.uid(), comm: bpf.comm() });
    return 0;
  }
}
`;

describe("bpf-ts compiler", () => {
  test("lowers tracepoints, maps, helpers and bounded loops to libbpf C", () => {
    const result = compileBpfTs(tracepointProgram, "exec.ts");
    expect(result.cSource).toContain('SEC("tracepoint/syscalls/sys_enter_execve")');
    expect(result.cSource).toContain("BPF_MAP_TYPE_RINGBUF");
    expect(result.cSource).toContain("BPF_MAP_TYPE_HASH");
    expect(result.cSource).toContain("bpf_get_current_pid_tgid()");
    expect(result.cSource).toContain("bpf_get_current_comm");
    expect(result.cSource).toContain("#pragma unroll");
    expect(result.cSource).toContain("__sync_fetch_and_add");
    expect(result.manifest.probes[0]).toEqual({
      name: "onExec",
      section: "tracepoint/syscalls/sys_enter_execve",
      kind: "tracepoint",
      category: "syscalls",
      event: "sys_enter_execve",
    });
  });

  test("preserves uprobe metadata, signed int arguments and clamps user-byte reads", () => {
    const result = compileBpfTs(`
interface Event { pid: u32; len: i32; sample: bytes<32>; }
const events = ringbuf<Event>(65536);
class TLSProbes {
  @uprobe("SSL_write")
  static capture(regs: UProbeContext): i32 {
    const buffer = bpf.arg(regs, 2);
    const length = bpf.argI32(regs, 3);
    if (length > 0) {
      events.emit({ pid: bpf.pid(), len: length, sample: bpf.userBytes(buffer, length) });
    }
    return 0;
  }
}
`, "tls.ts");
    expect(result.cSource).toContain('SEC("uprobe")');
    expect(result.cSource).toContain("struct pt_regs *regs");
    expect(result.cSource).toContain("__s32 length = PT_REGS_PARM3(regs);");
    expect(result.cSource).toContain("bpf_probe_read_user");
    expect(result.cSource).toContain("if (__bpf_ts_read_len_");
    expect(result.cSource).toContain("sizeof(__bpf_ts_event_");
    expect(result.manifest.probes[0]).toEqual({
      name: "capture",
      section: "uprobe",
      kind: "uprobe",
      target: "SSL_write",
    });
  });

  test("requires argI32 to be a direct local initializer", () => {
    expect(() => compileBpfTs(`
class TLSProbes {
  @uprobe("SSL_write")
  static capture(regs: UProbeContext): i32 {
    if (bpf.argI32(regs, 3) > 0) { return 0; }
    return 0;
  }
}
`, "nested-argi32.ts")).toThrow("bpf.argI32() must be used as a direct local initializer");
  });

  test("rejects argI32 in return probes", () => {
    expect(() => compileBpfTs(`
class TLSProbes {
  @uretprobe("SSL_write")
  static complete(regs: UProbeContext): i32 {
    const length = bpf.argI32(regs, 3);
    return 0;
  }
}
`, "return-argi32.ts")).toThrow("bpf.argI32() is not valid in uretprobe");
  });

  test("rejects unbounded while loops before C generation", () => {
    expect(() => compileBpfTs(`
class Bad {
  @kprobe("do_sys_open")
  static bad(ctx: KProbeContext): i32 {
    while (bpf.pid() > 0) { return 0; }
    return 0;
  }
}
`, "bad.ts")).toThrow(BpfTsCompileError);
  });

  test("rejects bounded loops larger than the verifier-safe policy", () => {
    expect(() => compileBpfTs(`
class Bad {
  @kprobe("do_sys_open")
  static bad(ctx: KProbeContext): i32 {
    for (let i = 0; i < 65; i++) { bpf.pid(); }
    return 0;
  }
}
`, "bad-loop.ts")).toThrow("maximum is 64");
  });

  test("rejects any and oversized byte arrays", () => {
    expect(() => compileBpfTs(`
interface Bad { value: any; }
class P { @kprobe("x") static p(ctx: KProbeContext): i32 { return 0; } }
`, "any.ts")).toThrow("'any' is not allowed");
    expect(() => compileBpfTs(`
interface Bad { data: bytes<4097>; }
class P { @kprobe("x") static p(ctx: KProbeContext): i32 { return 0; } }
`, "bytes.ts")).toThrow("between 1 and 4096");
  });

  test("explains why decorated top-level functions are not the DSL surface", () => {
    expect(() => compileBpfTs(`
@kprobe("x")
function p(ctx: KProbeContext): i32 { return 0; }
`, "free-function.ts")).toThrow(/top-level functions|TypeScript syntax error/);
  });
});