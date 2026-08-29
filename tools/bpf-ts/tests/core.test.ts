import { describe, expect, test } from "bun:test";
import { compileBpfTs } from "../src/compiler";

const coreProgram = `
declare interface task_struct { pid: i32; tgid: i32; }
interface Event { pid: i32; tgid: i32; current: u64; }
const events = ringbuf<Event>(65536);
class P {
  @kprobe("wake_up_new_task")
  static wake(ctx: KProbeContext): i32 {
    const task = bpf.arg(ctx, 1);
    const pid = bpf.coreRead.task_struct.pid(task);
    const tgid = bpf.coreRead.task_struct.tgid(task);
    events.emit({ pid, tgid, current: bpf.currentTask() });
    return 0;
  }
}
`;

describe("bpf-ts CO-RE", () => {
  test("lowers scalar kernel fields and preserves their inferred C types", () => {
    const result = compileBpfTs(coreProgram, "core.ts");
    expect(result.cSource).toContain("#include <bpf/bpf_core_read.h>");
    expect(result.cSource).toContain(
      "__s32 pid = BPF_CORE_READ((struct task_struct *)(unsigned long)(task), pid);",
    );
    expect(result.cSource).toContain(
      "__s32 tgid = BPF_CORE_READ((struct task_struct *)(unsigned long)(task), tgid);",
    );
    expect(result.cSource).toContain("bpf_get_current_task()");
  });

  test("requires declare interface for kernel BTF projections", () => {
    expect(() => compileBpfTs(`
interface task_struct { pid: i32; }
interface Event { pid: i32; }
const events = ringbuf<Event>(65536);
class P {
  @kprobe("x") static p(ctx: KProbeContext): i32 {
    const ptr = bpf.arg(ctx, 1);
    events.emit({ pid: bpf.coreRead.task_struct.pid(ptr) });
    return 0;
  }
}
`, "ordinary-interface.ts")).toThrow("must be declared as 'declare interface task_struct");
  });

  test("rejects using kernel projections directly as map values", () => {
    expect(() => compileBpfTs(`
declare interface task_struct { pid: i32; }
const events = ringbuf<task_struct>(65536);
class P { @kprobe("x") static p(ctx: KProbeContext): i32 { return 0; } }
`, "core-map.ts")).toThrow("cannot be used directly as map 'events' value");
  });

  test("rejects unknown kernel struct projections", () => {
    expect(() => compileBpfTs(`
interface Event { pid: u64; }
const events = ringbuf<Event>(65536);
class P {
  @kprobe("x") static p(ctx: KProbeContext): i32 {
    const ptr = bpf.arg(ctx, 1);
    events.emit({ pid: bpf.coreRead.missing.pid(ptr) });
    return 0;
  }
}
`, "missing-struct.ts")).toThrow("unknown kernel struct 'missing'");
  });

  test("rejects unknown and non-scalar CO-RE fields", () => {
    expect(() => compileBpfTs(`
declare interface task_struct { pid: i32; }
interface Event { pid: u64; }
const events = ringbuf<Event>(65536);
class P {
  @kprobe("x") static p(ctx: KProbeContext): i32 {
    const ptr = bpf.arg(ctx, 1);
    events.emit({ pid: bpf.coreRead.task_struct.tgid(ptr) });
    return 0;
  }
}
`, "missing-field.ts")).toThrow("unknown field 'task_struct.tgid'");

    expect(() => compileBpfTs(`
interface child { value: u64; }
declare interface task_struct { child: child; }
interface Event { value: u64; }
const events = ringbuf<Event>(65536);
class P {
  @kprobe("x") static p(ctx: KProbeContext): i32 {
    const ptr = bpf.arg(ctx, 1);
    events.emit({ value: bpf.coreRead.task_struct.child(ptr) });
    return 0;
  }
}
`, "non-scalar-field.ts")).toThrow("must be scalar");
  });

  test("rejects malformed CO-RE helper paths", () => {
    expect(() => compileBpfTs(`
declare interface task_struct { pid: i32; }
class P {
  @kprobe("x") static p(ctx: KProbeContext): i32 {
    const ptr = bpf.arg(ctx, 1);
    const value = bpf.coreRead.task_struct(ptr);
    return 0;
  }
}
`, "bad-core-path.ts")).toThrow("bpf.coreRead.<Struct>.<field>");
  });
});
