import { describe, expect, test } from "bun:test";
import { compileBpfTs } from "../src/compiler";

describe("bpf-ts verifier resource policy", () => {
  test("rejects byte-array locals that would silently consume BPF stack", () => {
    expect(() => compileBpfTs(`
class P {
  @kprobe("x") static p(ctx: KProbeContext): i32 {
    const local: bytes<16> = bpf.comm();
    return 0;
  }
}
`, "local-bytes.ts")).toThrow("only scalar local variables are supported");
  });

  test("keeps conservative headroom below the kernel BPF stack ceiling", () => {
    const locals = Array.from({ length: 49 }, (_, index) =>
      `const v${index}: u64 = bpf.ktimeNs();`,
    ).join("\n");
    expect(() => compileBpfTs(`
class P {
  @kprobe("x") static p(ctx: KProbeContext): i32 {
    ${locals}
    return 0;
  }
}
`, "stack-budget.ts")).toThrow("limits DSL locals to 384 bytes");
  });

  test("rejects nested unroll expansion beyond the compiler policy", () => {
    expect(() => compileBpfTs(`
class P {
  @kprobe("x") static p(ctx: KProbeContext): i32 {
    for (let i = 0; i < 64; i++) {
      for (let j = 0; j < 64; j++) {
        bpf.pid();
      }
    }
    return 0;
  }
}
`, "unroll-budget.ts")).toThrow("after loop unrolling");
  });
});
