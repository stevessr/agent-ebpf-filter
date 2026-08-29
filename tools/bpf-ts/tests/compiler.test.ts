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
      kind: "tracepoint",
      category: "syscalls",
      event: "sys_enter_execve",
    });
  });

  test("preserves uprobe attachment metadata and context identifiers", () => {
    const result = compileBpfTs(`
interface Event { pid: u32; len: u64; sample: bytes<32>; }
const events = ringbuf<Event>(65536);
class TLSProbes {
  @uprobe("SSL_write")
  static capture(regs: UProbeContext): i32 {
    const buffer = bpf.arg(regs, 2);
    events.emit({ pid: bpf.pid(), len: bpf.arg(regs, 3), sample: bpf.userBytes(buffer) });
    return 0;
  }
}
`, "tls.ts");
    expect(result.cSource).toContain('SEC("uprobe")');
    expect(result.cSource).toContain("struct pt_regs *regs");
    expect(result.cSource).toContain("PT_REGS_PARM3(regs)");
    expect(result.cSource).toContain("bpf_probe_read_user");
    expect(result.manifest.probes[0]).toEqual({
      name: "capture",
      kind: "uprobe",
      target: "SSL_write",
    });
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
