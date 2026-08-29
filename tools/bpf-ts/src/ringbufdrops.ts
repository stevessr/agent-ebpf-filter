import { BpfTsCompileError } from "./diagnostics";
import type { MapIR, ProgramIR } from "./ir";

export function ringbufDropMapName(ringbufName: string): string {
  return `__bpf_ts_drops_${ringbufName}`;
}

// Every user ringbuf gets one compiler-owned per-CPU u64 counter. The backend
// touches it only when reserve/output fails, keeping the successful hot path
// free of atomic operations while making kernel backpressure observable.
export function addRingbufDropMaps(program: ProgramIR): void {
  const occupied = new Set([
    ...program.maps.map((map) => map.name),
    ...program.probes.map((probe) => probe.name),
  ]);
  const additions: MapIR[] = [];

  for (const ringbuf of [...program.maps]) {
    if (ringbuf.kind !== "ringbuf") continue;
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
