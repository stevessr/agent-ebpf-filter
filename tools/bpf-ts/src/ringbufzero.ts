import type { ProgramIR } from "./ir";

// LLVM's BPF backend cannot lower a large surviving memset intrinsic. Small
// event structs are already expanded into stores by clang, but a 4 KiB TLS
// preview leaves a call to memset and compilation fails. Keep the ordinary
// fast path for small programs and rewrite generated event initialization only
// when the source contains a large byte-array field.
const largeInlineMemsetThreshold = 256;

function hasLargeRingbufValue(program: ProgramIR): boolean {
  const structs = new Map(program.structs.map((item) => [item.name, item]));
  for (const map of program.maps) {
    if (map.kind !== "ringbuf" || map.valueType.kind !== "named") continue;
    const struct = structs.get(map.valueType.name);
    if (!struct) continue;
    if (struct.fields.some((field) => field.type.kind === "bytes" && field.type.length > largeInlineMemsetThreshold)) {
      return true;
    }
  }
  return false;
}

// The rewrite zeros the complete reserved record, including C struct padding,
// before any field is populated. Eight-byte stores keep the runtime cost much
// lower than a byte-at-a-time 4 KiB loop; a short byte tail handles arbitrary
// struct sizes safely. Both loops have compile-time sizeof bounds and are kept
// rolled so generated instruction count stays bounded.
export function lowerLargeRingbufZeroing(program: ProgramIR, cSource: string): string {
  if (!hasLargeRingbufValue(program)) return cSource;

  return cSource.replace(
    /^(\s*)__builtin_memset\((__bpf_ts_event_\d+), 0, sizeof\(\*\2\)\);$/gm,
    (_match, indent: string, event: string) => {
      const offset = `${event}_zero_off`;
      return [
        `${indent}__u32 ${offset} = 0;`,
        `${indent}#pragma clang loop unroll(disable)`,
        `${indent}for (; ${offset} + sizeof(__u64) <= sizeof(*${event}); ${offset} += sizeof(__u64)) {`,
        `${indent}  *(__u64 *)((unsigned char *)${event} + ${offset}) = 0;`,
        `${indent}}`,
        `${indent}#pragma clang loop unroll(disable)`,
        `${indent}for (; ${offset} < sizeof(*${event}); ${offset}++) {`,
        `${indent}  ((unsigned char *)${event})[${offset}] = 0;`,
        `${indent}}`,
      ].join("\n");
    },
  );
}
