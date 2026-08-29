import { BpfTsCompileError } from "./diagnostics";
import type { BpfType, ExprIR, MapIR, ProbeIR, ProgramIR, StmtIR } from "./ir";

const expressionHelpers = new Set([
  "bpf.pid",
  "bpf.tid",
  "bpf.uid",
  "bpf.gid",
  "bpf.ktimeNs",
  "bpf.arg",
]);
const ringbufByteHelpers = new Set([
  "bpf.comm",
  "bpf.userString",
  "bpf.userBytes",
]);
const statementHelpers = new Set(["bpf.readUser"]);

function isPowerOfTwo(value: number) {
  return value > 0 && (value & (value - 1)) === 0;
}

function validateKnownType(type: BpfType, structs: Set<string>, where: string) {
  if (type.kind === "named" && !structs.has(type.name)) {
    throw new BpfTsCompileError(`${where} references unknown struct '${type.name}'`);
  }
}

function validateMap(map: MapIR, structs: Set<string>) {
  validateKnownType(map.valueType, structs, `map '${map.name}' value type`);
  if (map.keyType) validateKnownType(map.keyType, structs, `map '${map.name}' key type`);

  if (map.kind === "ringbuf") {
    if (map.valueType.kind !== "named") {
      throw new BpfTsCompileError(`ringbuf '${map.name}' must use a named struct value type`);
    }
    // Linux ring-buffer maps require a power-of-two capacity. Keeping the
    // minimum at one page also avoids declarations that can never be loaded on
    // the repository's supported Linux targets.
    if (map.maxEntries < 4096 || !isPowerOfTwo(map.maxEntries)) {
      throw new BpfTsCompileError(
        `ringbuf '${map.name}' capacity must be a power of two and at least 4096 bytes`,
      );
    }
    return;
  }

  if (map.keyType?.kind === "bytes" || map.valueType.kind === "bytes") {
    throw new BpfTsCompileError(
      `map '${map.name}' cannot use bytes<N> directly as a key/value; wrap fixed bytes in a named struct`,
    );
  }
}

function validateStructCycles(program: ProgramIR) {
  const structs = new Map(program.structs.map((struct) => [struct.name, struct]));
  const visiting = new Set<string>();
  const visited = new Set<string>();

  const visit = (name: string, path: string[]) => {
    if (visited.has(name)) return;
    if (visiting.has(name)) {
      throw new BpfTsCompileError(
        `recursive by-value BPF struct layout is not supported: ${[...path, name].join(" -> ")}`,
      );
    }
    const struct = structs.get(name);
    if (!struct) return;
    visiting.add(name);
    for (const field of struct.fields) {
      if (field.type.kind === "named") visit(field.type.name, [...path, name]);
    }
    visiting.delete(name);
    visited.add(name);
  };

  for (const name of structs.keys()) visit(name, []);
}

function walkExpr(expr: ExprIR, visit: (expr: ExprIR) => void) {
  visit(expr);
  switch (expr.kind) {
    case "property":
      walkExpr(expr.object, visit);
      break;
    case "binary":
      walkExpr(expr.left, visit);
      walkExpr(expr.right, visit);
      break;
    case "unary":
      walkExpr(expr.value, visit);
      break;
    case "call":
      for (const arg of expr.args) walkExpr(arg, visit);
      break;
    case "object":
      for (const field of expr.fields) walkExpr(field.value, visit);
      break;
    default:
      break;
  }
}

function walkStatements(statements: StmtIR[], visit: (expr: ExprIR) => void) {
  for (const stmt of statements) {
    switch (stmt.kind) {
      case "let":
        walkExpr(stmt.value, visit);
        break;
      case "assign":
        walkExpr(stmt.target, visit);
        walkExpr(stmt.value, visit);
        break;
      case "expr":
      case "return":
        walkExpr(stmt.value, visit);
        break;
      case "if":
        walkExpr(stmt.test, visit);
        walkStatements(stmt.then, visit);
        walkStatements(stmt.otherwise, visit);
        break;
      case "for":
        walkStatements(stmt.body, visit);
        break;
    }
  }
}

function validateProbeContext(probe: ProbeIR) {
  const expected =
    probe.attach.kind === "kprobe"
      ? "KProbeContext"
      : probe.attach.kind === "uprobe"
        ? "UProbeContext"
        : "TracepointContext";
  if (probe.contextType !== expected && probe.contextType !== "ProbeContext") {
    throw new BpfTsCompileError(
      `probe '${probe.name}' uses ${probe.contextType}; ${probe.attach.kind} expects ${expected} or ProbeContext`,
    );
  }
}

function validateCall(probe: ProbeIR, expr: Extract<ExprIR, { kind: "call" }>, maps: Map<string, MapIR>) {
  if (expressionHelpers.has(expr.callee)) {
    if (expr.callee === "bpf.arg") {
      if (probe.attach.kind === "tracepoint") {
        throw new BpfTsCompileError(`bpf.arg() is not valid in tracepoint probe '${probe.name}'`);
      }
      if (
        expr.args.length !== 2 ||
        expr.args[0].kind !== "identifier" ||
        expr.args[0].name !== probe.contextName
      ) {
        throw new BpfTsCompileError(
          `bpf.arg() in probe '${probe.name}' must use its context parameter '${probe.contextName}' as the first argument`,
        );
      }
    }
    return;
  }
  if (ringbufByteHelpers.has(expr.callee) || statementHelpers.has(expr.callee)) return;

  const dot = expr.callee.lastIndexOf(".");
  if (dot > 0) {
    const mapName = expr.callee.slice(0, dot);
    const method = expr.callee.slice(dot + 1);
    const map = maps.get(mapName);
    if (map) {
      if (map.kind === "ringbuf" && method === "emit") return;
      if (map.kind !== "ringbuf" && (method === "set" || method === "increment")) return;
      throw new BpfTsCompileError(`map operation '${expr.callee}' is not valid for ${map.kind} map '${mapName}'`);
    }
  }
  throw new BpfTsCompileError(`unknown or unsupported call '${expr.callee}' in probe '${probe.name}'`);
}

function validateProbe(probe: ProbeIR, maps: Map<string, MapIR>) {
  validateProbeContext(probe);
  if (probe.body.length === 0 || probe.body[probe.body.length - 1].kind !== "return") {
    throw new BpfTsCompileError(`probe '${probe.name}' must end with an explicit return statement`);
  }
  walkStatements(probe.body, (expr) => {
    if (expr.kind === "call") validateCall(probe, expr, maps);
  });
}

export function validateBpfProgram(program: ProgramIR) {
  const structNames = new Set<string>();
  for (const struct of program.structs) {
    if (structNames.has(struct.name)) {
      throw new BpfTsCompileError(`duplicate BPF struct '${struct.name}'`);
    }
    structNames.add(struct.name);
  }
  for (const struct of program.structs) {
    for (const field of struct.fields) {
      validateKnownType(field.type, structNames, `struct '${struct.name}' field '${field.name}'`);
    }
  }
  validateStructCycles(program);

  const maps = new Map<string, MapIR>();
  for (const map of program.maps) {
    if (maps.has(map.name)) throw new BpfTsCompileError(`duplicate BPF map '${map.name}'`);
    maps.set(map.name, map);
    validateMap(map, structNames);
  }

  const probes = new Set<string>();
  for (const probe of program.probes) {
    if (probes.has(probe.name)) throw new BpfTsCompileError(`duplicate BPF probe '${probe.name}'`);
    if (maps.has(probe.name)) {
      throw new BpfTsCompileError(`BPF probe '${probe.name}' conflicts with map '${probe.name}' in generated C`);
    }
    probes.add(probe.name);
    validateProbe(probe, maps);
  }
}
