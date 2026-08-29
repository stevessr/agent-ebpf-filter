import { BpfTsCompileError } from "./diagnostics";
import type { ExprIR, MapIR, ProgramIR, StmtIR } from "./ir";

export function ringbufScratchMapName(ringbufName: string): string {
  return `__bpf_ts_scratch_${ringbufName}`;
}

function isTrailingUserBytesEmit(program: ProgramIR, ringbuf: MapIR, expr: ExprIR): boolean {
  if (expr.kind !== "call" || expr.callee !== `${ringbuf.name}.emit` || expr.args.length !== 1) {
    return false;
  }
  const payload = expr.args[0];
  if (payload.kind !== "object" || ringbuf.valueType.kind !== "named") return false;
  const struct = program.structs.find((candidate) => candidate.name === ringbuf.valueType.name);
  const tail = struct?.fields.at(-1);
  if (!tail || tail.type.kind !== "bytes") return false;
  const field = payload.fields.find((candidate) => candidate.name === tail.name);
  return field?.value.kind === "call" && field.value.callee === "bpf.userBytes" && field.value.args.length === 2;
}

function statementsUseCompactRingbuf(program: ProgramIR, ringbuf: MapIR, statements: StmtIR[]): boolean {
  for (const statement of statements) {
    switch (statement.kind) {
      case "expr":
        if (isTrailingUserBytesEmit(program, ringbuf, statement.value)) return true;
        break;
      case "if":
        if (
          statementsUseCompactRingbuf(program, ringbuf, statement.then) ||
          statementsUseCompactRingbuf(program, ringbuf, statement.otherwise)
        ) {
          return true;
        }
        break;
      case "for":
        if (statementsUseCompactRingbuf(program, ringbuf, statement.body)) return true;
        break;
      default:
        break;
    }
  }
  return false;
}

// Add compiler-owned per-CPU struct storage for ringbuf records whose final
// bytes<N> field is sourced from bpf.userBytes(). The C backend can then fill
// the map value and call bpf_ringbuf_output() with only header+captured bytes,
// avoiding a large fixed reserve/memset and any uninitialized tail disclosure.
//
// This pass intentionally runs after user-program validation. The synthetic map
// is constructed only from already-validated ringbuf/struct metadata and is
// still included in the manifest for an exact ELF contract.
export function addCompactRingbufScratchMaps(program: ProgramIR): void {
  const occupied = new Set([
    ...program.maps.map((map) => map.name),
    ...program.probes.map((probe) => probe.name),
  ]);
  const additions: MapIR[] = [];
  for (const ringbuf of program.maps) {
    if (ringbuf.kind !== "ringbuf" || ringbuf.valueType.kind !== "named") continue;
    const compact = program.probes.some((probe) =>
      statementsUseCompactRingbuf(program, ringbuf, probe.body),
    );
    if (!compact) continue;

    const name = ringbufScratchMapName(ringbuf.name);
    if (occupied.has(name)) {
      throw new BpfTsCompileError(
        `compiler scratch map '${name}' conflicts with a user BPF identifier`,
      );
    }
    occupied.add(name);
    additions.push({
      name,
      kind: "percpu_array",
      keyType: { kind: "scalar", name: "u32" },
      valueType: ringbuf.valueType,
      maxEntries: 1,
    });
  }
  program.maps.push(...additions);
}
