import { BpfTsCompileError } from "./diagnostics";
import { ringbufScratchMapName } from "./ringbufscratch";
import type { MapIR, ProgramIR } from "./ir";

export function ringbufDropMapName(ringbufName: string): string {
  return `__bpf_ts_drops_${ringbufName}`;
}

// Compact ringbufs get one compiler-owned per-CPU u64 counter. The backend
// checks bpf_ringbuf_output() and touches the counter only on failure, so the
// successful hot path adds no atomic operation. Small reserve/submit ringbufs
// intentionally remain unchanged until their verifier reference semantics are
// covered by a dedicated kernel gate.
export function addRingbufDropMaps(program: ProgramIR): void {
  const occupied = new Set([
    ...program.maps.map((map) => map.name),
    ...program.probes.map((probe) => probe.name),
  ]);
  const additions: MapIR[] = [];

  for (const ringbuf of [...program.maps]) {
    if (ringbuf.kind !== "ringbuf") continue;
    const scratch = program.maps.find((map) => map.name === ringbufScratchMapName(ringbuf.name));
    if (!scratch || scratch.kind !== "percpu_array") continue;

    const name = ringbufDropMapName(ringbuf.name);
    if (occupied.has(name)) {
      throw new BpfTsCompileError(
        `compiler drop map '${name}' conflicts with a BPF identifier`,
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
