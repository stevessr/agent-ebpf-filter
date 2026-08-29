import { describe, expect, test } from "bun:test";
import { compileBpfTs } from "../src/compiler";

function probe(body: string, decorator = '@kprobe("do_sys_open")', context = "KProbeContext") {
  return `
class Probes {
  ${decorator}
  static run(ctx: ${context}): i32 {
    ${body}
  }
}
`;
}

describe("bpf-ts semantic validation", () => {
  test("rejects ringbuf capacities the kernel cannot load", () => {
    expect(() => compileBpfTs(`
interface Event { pid: u32; }
const events = ringbuf<Event>(6000);
${probe("events.emit({ pid: bpf.pid() }); return 0;")}
`)).toThrow("power of two");

    expect(() => compileBpfTs(`
interface Event { pid: u32; }
const events = ringbuf<Event>(2048);
${probe("events.emit({ pid: bpf.pid() }); return 0;")}
`)).toThrow("at least 4096");
  });

  test("rejects unknown and recursive by-value struct layouts", () => {
    expect(() => compileBpfTs(`
interface Event { nested: Missing; }
const events = ringbuf<Event>(4096);
${probe("return 0;")}
`)).toThrow("unknown struct 'Missing'");

    expect(() => compileBpfTs(`
interface A { b: B; }
interface B { a: A; }
const events = ringbuf<A>(4096);
${probe("return 0;")}
`)).toThrow("recursive by-value BPF struct layout");
  });

  test("rejects probe context mismatches", () => {
    expect(() => compileBpfTs(probe("return 0;", '@uprobe("SSL_write")', "TracepointContext"))).toThrow(
      "uprobe expects UProbeContext",
    );
  });

  test("rejects register arguments in tracepoints", () => {
    expect(() => compileBpfTs(probe(
      "const value = bpf.arg(ctx, 1); return 0;",
      '@tracepoint("syscalls", "sys_enter_execve")',
      "TracepointContext",
    ))).toThrow("bpf.arg() is not valid in tracepoint");
  });

  test("requires bpf.arg to use the current probe context", () => {
    expect(() => compileBpfTs(`
class Probes {
  @uprobe("SSL_write")
  static run(regs: UProbeContext): i32 {
    const value = bpf.arg(ctx, 2);
    return 0;
  }
}
`)).toThrow("must use its context parameter 'regs'");
  });

  test("rejects unknown calls rather than emitting arbitrary C", () => {
    expect(() => compileBpfTs(probe("dangerousRuntimeCall(); return 0;"))).toThrow(
      "unknown or unsupported call 'dangerousRuntimeCall'",
    );
  });

  test("requires an explicit final return", () => {
    expect(() => compileBpfTs(probe("const pid = bpf.pid();"))).toThrow(
      "must end with an explicit return statement",
    );
  });

  test("rejects map/probe identifier collisions in generated C", () => {
    expect(() => compileBpfTs(`
const run = hash<u32, u64>(16);
class Probes {
  @kprobe("do_sys_open")
  static run(ctx: KProbeContext): i32 { return 0; }
}
`)).toThrow("conflicts with map 'run'");
  });
});
