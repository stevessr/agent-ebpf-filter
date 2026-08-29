import { describe, expect, test } from "bun:test";
import { bpfTypeSize } from "../src/abi";
import { compileBpfTs } from "../src/compiler";
import type { ProgramIR } from "../src/ir";

describe("bpf-ts map ABI", () => {
  test("computes natural C layout for nested fixed structs", () => {
    const program: ProgramIR = {
      structs: [
        {
          name: "Inner",
          fields: [
            { name: "flag", type: { kind: "scalar", name: "u8" } },
            { name: "value", type: { kind: "scalar", name: "u64" } },
          ],
        },
        {
          name: "Outer",
          fields: [
            { name: "tag", type: { kind: "scalar", name: "u16" } },
            { name: "inner", type: { kind: "named", name: "Inner" } },
            { name: "tail", type: { kind: "bytes", length: 3 } },
          ],
        },
      ],
      maps: [],
      probes: [],
    };
    expect(bpfTypeSize(program, { kind: "named", name: "Inner" })).toBe(16);
    expect(bpfTypeSize(program, { kind: "named", name: "Outer" })).toBe(32);
  });

  test("emits scalar map key/value widths", () => {
    const result = compileBpfTs(`
const counts = hash<u32, u64>(1024);
class P {
  @kprobe("do_sys_open")
  static p(ctx: KProbeContext): i32 {
    counts.set(bpf.pid(), 1);
    return 0;
  }
}
`, "map-abi.ts");
    expect(result.manifest.maps).toContainEqual({
      name: "counts",
      kind: "hash",
      maxEntries: 1024,
      keySize: 4,
      valueSize: 8,
    });
  });

  test("locks the OpenSSL compiler-owned scratch and counter widths", async () => {
    const source = await Bun.file(new URL("../examples/tls-openssl.ts", import.meta.url)).text();
    const result = compileBpfTs(source, "tls-openssl.ts");
    expect(result.manifest.maps).toContainEqual({
      name: "__bpf_ts_scratch_tlsOpenSSLEvents",
      kind: "percpu_array",
      maxEntries: 1,
      keySize: 4,
      valueSize: 4144,
    });
    expect(result.manifest.maps).toContainEqual({
      name: "__bpf_ts_drops_tlsOpenSSLEvents",
      kind: "percpu_array",
      maxEntries: 1,
      keySize: 4,
      valueSize: 8,
    });
    expect(result.manifest.maps).toContainEqual({
      name: "__bpf_ts_read_errors_tlsOpenSSLEvents",
      kind: "percpu_array",
      maxEntries: 1,
      keySize: 4,
      valueSize: 8,
    });
  });
});
