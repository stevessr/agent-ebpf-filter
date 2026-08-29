import { BpfTsCompileError } from "./diagnostics";
import type { BpfType, ExprIR, MapIR, ProbeIR, ProgramIR, StmtIR } from "./ir";

const zeroArgHelpers = new Set([
  "bpf.pid",
  "bpf.tid",
  "bpf.uid",
  "bpf.gid",
  "bpf.ktimeNs",
  "bpf.comm",
  "bpf.currentTask",
]);

interface Scope {
  defined: Set<string>;
  readOnly: Set<string>;
}

function cloneScope(scope: Scope): Scope {
  return { defined: new Set(scope.defined), readOnly: new Set(scope.readOnly) };
}

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
    if (map.maxEntries < 4096 || !isPowerOfTwo(map.maxEntries)) {
      throw new BpfTsCompileError(`ringbuf '${map.name}' capacity must be a power of two and at least 4096 bytes`);
    }
    return;
  }
  if (map.keyType?.kind !== "scalar" || map.valueType.kind !== "scalar") {
    throw new BpfTsCompileError(`map '${map.name}' currently requires scalar key/value types; use a ringbuf for structured events`);
  }
}

function validateStructCycles(program: ProgramIR) {
  const structs = new Map(program.structs.map((struct) => [struct.name, struct]));
  const visiting = new Set<string>();
  const visited = new Set<string>();
  const visit = (name: string, path: string[]) => {
    if (visited.has(name)) return;
    if (visiting.has(name)) {
      throw new BpfTsCompileError(`recursive by-value BPF struct layout is not supported: ${[...path, name].join(" -> ")}`);
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

function requireArity(expr: Extract<ExprIR, { kind: "call" }>, expected: number) {
  if (expr.args.length !== expected) {
    throw new BpfTsCompileError(`${expr.callee}() expects ${expected} argument${expected === 1 ? "" : "s"}`);
  }
}

function isReturnProbe(probe: ProbeIR) {
  return probe.attach.kind === "kretprobe" || probe.attach.kind === "uretprobe";
}

function validateProbeContext(probe: ProbeIR) {
  const expected =
    probe.attach.kind === "kprobe" || probe.attach.kind === "kretprobe"
      ? "KProbeContext"
      : probe.attach.kind === "uprobe" || probe.attach.kind === "uretprobe"
        ? "UProbeContext"
        : "TracepointContext";
  if (probe.contextType !== expected && probe.contextType !== "ProbeContext") {
    throw new BpfTsCompileError(`probe '${probe.name}' uses ${probe.contextType}; ${probe.attach.kind} expects ${expected} or ProbeContext`);
  }
}

function validateContextHelper(probe: ProbeIR, expr: Extract<ExprIR, { kind: "call" }>, helper: "bpf.ret" | "bpf.retI32") {
  requireArity(expr, 1);
  if (!isReturnProbe(probe)) {
    throw new BpfTsCompileError(`${helper}() is only valid in kretprobe/uretprobe probe '${probe.name}'`);
  }
  if (expr.args[0].kind !== "identifier" || expr.args[0].name !== probe.contextName) {
    throw new BpfTsCompileError(`${helper}() in probe '${probe.name}' must use its context parameter '${probe.contextName}'`);
  }
}

function validateCall(probe: ProbeIR, expr: Extract<ExprIR, { kind: "call" }>, maps: Map<string, MapIR>) {
  if (zeroArgHelpers.has(expr.callee)) {
    requireArity(expr, 0);
    return;
  }
  if (expr.callee === "bpf.ret" || expr.callee === "bpf.retI32") {
    validateContextHelper(probe, expr, expr.callee);
    return;
  }
  if (expr.callee.startsWith("bpf.coreRead.")) {
    requireArity(expr, 1);
    return;
  }
  if (expr.callee === "bpf.arg") {
    requireArity(expr, 2);
    if (probe.attach.kind === "tracepoint" || isReturnProbe(probe)) {
      throw new BpfTsCompileError(`bpf.arg() is not valid in ${probe.attach.kind} probe '${probe.name}'`);
    }
    if (expr.args[0].kind !== "identifier" || expr.args[0].name !== probe.contextName) {
      throw new BpfTsCompileError(`bpf.arg() in probe '${probe.name}' must use its context parameter '${probe.contextName}' as the first argument`);
    }
    const index = expr.args[1];
    if (index.kind !== "number" || !Number.isInteger(index.value) || index.value < 1 || index.value > 5) {
      throw new BpfTsCompileError("bpf.arg() argument index must be an integer from 1 through 5");
    }
    return;
  }
  if (expr.callee === "bpf.userString") {
    requireArity(expr, 1);
    return;
  }
  if (expr.callee === "bpf.userBytes" || expr.callee === "bpf.readUser") {
    requireArity(expr, 2);
    return;
  }

  const dot = expr.callee.lastIndexOf(".");
  if (dot > 0) {
    const mapName = expr.callee.slice(0, dot);
    const method = expr.callee.slice(dot + 1);
    const map = maps.get(mapName);
    if (map) {
      if (map.kind === "ringbuf" && method === "emit") {
        requireArity(expr, 1);
        return;
      }
      if (map.kind !== "ringbuf" && method === "set") {
        requireArity(expr, 2);
        return;
      }
      if (map.kind !== "ringbuf" && method === "getOr") {
        requireArity(expr, 2);
        return;
      }
      if (map.kind === "hash" && method === "takeOr") {
        requireArity(expr, 2);
        return;
      }
      if (map.kind === "hash" && method === "delete") {
        requireArity(expr, 1);
        return;
      }
      if (map.kind !== "ringbuf" && method === "increment") {
        requireArity(expr, 1);
        if (map.valueType.kind !== "scalar" || map.valueType.name !== "u64") {
          throw new BpfTsCompileError(`${mapName}.increment() requires a u64 map value type`);
        }
        return;
      }
      throw new BpfTsCompileError(`map operation '${expr.callee}' is not valid for ${map.kind} map '${mapName}'`);
    }
  }
  throw new BpfTsCompileError(`unknown or unsupported call '${expr.callee}' in probe '${probe.name}'`);
}

function validateExpr(probe: ProbeIR, expr: ExprIR, maps: Map<string, MapIR>, scope: Scope) {
  switch (expr.kind) {
    case "number":
      if (!Number.isSafeInteger(expr.value)) {
        throw new BpfTsCompileError(`bpf-ts numeric literals must be safe integers; received '${expr.value}' in probe '${probe.name}'`);
      }
      return;
    case "boolean":
      return;
    case "identifier":
      if (!scope.defined.has(expr.name)) throw new BpfTsCompileError(`unknown identifier '${expr.name}' in probe '${probe.name}'`);
      return;
    case "property":
      throw new BpfTsCompileError(`property access '${expr.property}' is not supported yet; use explicit BPF helpers instead`);
    case "binary":
      validateExpr(probe, expr.left, maps, scope);
      validateExpr(probe, expr.right, maps, scope);
      return;
    case "unary":
      validateExpr(probe, expr.value, maps, scope);
      return;
    case "call":
      validateCall(probe, expr, maps);
      for (const arg of expr.args) validateExpr(probe, arg, maps, scope);
      return;
    case "object": {
      const fields = new Set<string>();
      for (const field of expr.fields) {
        if (fields.has(field.name)) throw new BpfTsCompileError(`duplicate object field '${field.name}' in probe '${probe.name}'`);
        fields.add(field.name);
        validateExpr(probe, field.value, maps, scope);
      }
    }
  }
}

function validateStatements(probe: ProbeIR, statements: StmtIR[], maps: Map<string, MapIR>, scope: Scope) {
  for (const stmt of statements) {
    switch (stmt.kind) {
      case "let":
        validateExpr(probe, stmt.value, maps, scope);
        if (scope.defined.has(stmt.name)) throw new BpfTsCompileError(`local '${stmt.name}' shadows an existing identifier in probe '${probe.name}'`);
        scope.defined.add(stmt.name);
        break;
      case "assign":
        if (stmt.target.kind !== "identifier") throw new BpfTsCompileError("assignment targets must be local scalar identifiers");
        if (!scope.defined.has(stmt.target.name)) throw new BpfTsCompileError(`assignment to unknown identifier '${stmt.target.name}'`);
        if (scope.readOnly.has(stmt.target.name)) throw new BpfTsCompileError(`cannot assign to read-only identifier '${stmt.target.name}'`);
        validateExpr(probe, stmt.value, maps, scope);
        break;
      case "expr":
      case "return":
        validateExpr(probe, stmt.value, maps, scope);
        break;
      case "if":
        validateExpr(probe, stmt.test, maps, scope);
        validateStatements(probe, stmt.then, maps, cloneScope(scope));
        validateStatements(probe, stmt.otherwise, maps, cloneScope(scope));
        break;
      case "for": {
        const loopScope = cloneScope(scope);
        if (loopScope.defined.has(stmt.variable)) throw new BpfTsCompileError(`loop variable '${stmt.variable}' shadows an existing identifier`);
        loopScope.defined.add(stmt.variable);
        loopScope.readOnly.add(stmt.variable);
        validateStatements(probe, stmt.body, maps, loopScope);
        break;
      }
    }
  }
}

function validateProbe(probe: ProbeIR, maps: Map<string, MapIR>) {
  validateProbeContext(probe);
  if (probe.body.length === 0 || probe.body[probe.body.length - 1].kind !== "return") {
    throw new BpfTsCompileError(`probe '${probe.name}' must end with an explicit return statement`);
  }
  validateStatements(probe, probe.body, maps, {
    defined: new Set([probe.contextName]),
    readOnly: new Set([probe.contextName]),
  });
}

export function validateBpfProgram(program: ProgramIR) {
  const structNames = new Set<string>();
  for (const struct of program.structs) {
    if (structNames.has(struct.name)) throw new BpfTsCompileError(`duplicate BPF struct '${struct.name}'`);
    structNames.add(struct.name);
  }
  for (const struct of program.structs) {
    const fields = new Set<string>();
    for (const field of struct.fields) {
      if (fields.has(field.name)) throw new BpfTsCompileError(`duplicate field '${field.name}' in BPF struct '${struct.name}'`);
      fields.add(field.name);
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
    if (maps.has(probe.name)) throw new BpfTsCompileError(`BPF probe '${probe.name}' conflicts with map '${probe.name}' in generated C`);
    probes.add(probe.name);
    validateProbe(probe, maps);
  }
}
