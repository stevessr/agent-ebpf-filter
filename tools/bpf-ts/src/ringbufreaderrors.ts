import { BpfTsCompileError } from "./diagnostics";
import { ringbufScratchMapName } from "./ringbufscratch";
import type { MapIR, ProgramIR } from "./ir";

export function ringbufReadErrorMapName(ringbufName: string): string {
  return `__bpf_ts_read_errors_${ringbufName}`;
}

// Compact userBytes ringbufs suppress a record when bpf_probe_read_user fails.
// Add a separate per-CPU counter so those failures are not confused with
// bpf_ringbuf_output backpressure drops.
export function addRingbufReadErrorMaps(program: ProgramIR): void {
  const occupied = new Set([
    ...program.maps.map((map) => map.name),
    ...program.probes.map((probe) => probe.name),
  ]);
  const additions: MapIR[] = [];

  for (const ringbuf of [...program.maps]) {
    if (ringbuf.kind !== "ringbuf") continue;
    const scratch = program.maps.find((map) => map.name === ringbufScratchMapName(ringbuf.name));
    if (!scratch || scratch.kind !== "percpu_array") continue;

    const name = ringbufReadErrorMapName(ringbuf.name);
    if (occupied.has(name)) {
      throw new BpfTsCompileError(
        `compiler read-error map '${name}' conflicts with a BPF identifier`,
      );
    }
    occupied.add(name);
    additions.push({
      name,
      kind: "percpu_array",
      keyType: { kind: "scalar", name: "u32" },
      valueType: { kind: "scalar", name: "u64" },
      maxEntries: 1,
    });
  }

  program.maps.push(...additions);
}
