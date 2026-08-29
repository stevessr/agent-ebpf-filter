import { ringbufDropMapName } from "./ringbufdrops";
import type { ProgramIR } from "./ir";

// Rewrite compact bpf_ringbuf_output calls through an always-inline helper that
// records output failures in a per-CPU counter. The helper is injected after map
// definitions, so both the user ringbuf and compiler-owned counter are visible.
// Small reserve/submit ringbufs are deliberately untouched.
export function instrumentCompactRingbufDrops(program: ProgramIR, cSource: string): string {
  const ringbufs = program.maps.filter((map) => map.kind === "ringbuf");
  const dropNames = new Set(
    program.maps
      .filter((map) => map.kind === "percpu_array")
      .map((map) => map.name),
  );

  const helpers: string[] = [];
  let output = cSource;
  for (const ringbuf of ringbufs) {
    const dropMap = ringbufDropMapName(ringbuf.name);
    if (!dropNames.has(dropMap)) continue;

    const noteHelper = `__bpf_ts_note_drop_${ringbuf.name}`;
    const outputHelper = `__bpf_ts_output_${ringbuf.name}`;
    helpers.push(
      `static __always_inline void ${noteHelper}(void) {`,
      "  __u32 key = 0;",
      `  __u64 *counter = bpf_map_lookup_elem(&${dropMap}, &key);`,
      "  if (counter) (*counter)++;",
      "}",
      "",
      // bpf_ringbuf_output() takes a mutable void* in libbpf's helper ABI even
      // though it does not mutate the payload. Match that declaration exactly
      // so generated code remains warning-clean on both target ABIs.
      `static __always_inline long ${outputHelper}(void *data, __u64 size, __u64 flags) {`,
      `  long rc = bpf_ringbuf_output(&${ringbuf.name}, data, size, flags);`,
      `  if (rc != 0) ${noteHelper}();`,
      "  return rc;",
      "}",
      "",
    );

    output = output.split(`bpf_ringbuf_output(&${ringbuf.name}, `).join(`${outputHelper}(`);
  }

  if (helpers.length === 0) return output;
  const firstProbe = output.indexOf("// bpf-ts attach:");
  if (firstProbe < 0) return output;
  return `${output.slice(0, firstProbe)}${helpers.join("\n")}\n${output.slice(firstProbe)}`;
}
