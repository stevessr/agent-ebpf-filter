import { BpfTsCompileError } from "./diagnostics";
import type { ExprIR, MapIR, ProgramIR, StmtIR, StructIR } from "./ir";

function walkStatements(statements: StmtIR[], visitCall: (call: Extract<ExprIR, { kind: "call" }>) => void) {
  const walkExpr = (expr: ExprIR) => {
    switch (expr.kind) {
      case "call":
        visitCall(expr);
        for (const arg of expr.args) walkExpr(arg);
        break;
      case "property":
        walkExpr(expr.object);
        break;
      case "binary":
        walkExpr(expr.left);
        walkExpr(expr.right);
        break;
      case "unary":
        walkExpr(expr.value);
        break;
      case "object":
        for (const field of expr.fields) walkExpr(field.value);
        break;
      default:
        break;
    }
  };

  for (const statement of statements) {
    switch (statement.kind) {
      case "let":
        walkExpr(statement.value);
        break;
      case "assign":
        walkExpr(statement.target);
        walkExpr(statement.value);
        break;
      case "expr":
      case "return":
        walkExpr(statement.value);
        break;
      case "if":
        walkExpr(statement.test);
        walkStatements(statement.then, visitCall);
        walkStatements(statement.otherwise, visitCall);
        break;
      case "for":
        walkStatements(statement.body, visitCall);
        break;
    }
  }
}

function validateRingbufEmit(map: MapIR, struct: StructIR, call: Extract<ExprIR, { kind: "call" }>) {
  if (call.args.length !== 1 || call.args[0].kind !== "object") {
    throw new BpfTsCompileError(`${map.name}.emit() requires one object literal matching struct ${struct.name}`);
  }
  const payload = call.args[0];
  const expected = new Set(struct.fields.map((field) => field.name));
  const seen = new Set<string>();
  for (const field of payload.fields) {
    if (!expected.has(field.name)) {
      throw new BpfTsCompileError(`struct ${struct.name} has no field '${field.name}'`);
    }
    if (seen.has(field.name)) {
      throw new BpfTsCompileError(`duplicate field '${field.name}' in ${map.name}.emit() payload`);
    }
    seen.add(field.name);
  }
  const missing = struct.fields.filter((field) => !seen.has(field.name)).map((field) => field.name);
  if (missing.length > 0) {
    throw new BpfTsCompileError(
      `${map.name}.emit() payload is missing required field${missing.length === 1 ? "" : "s"}: ${missing.join(", ")}`,
    );
  }
}

export function validatePayloadShapes(program: ProgramIR) {
  const maps = new Map(program.maps.map((map) => [map.name, map]));
  const structs = new Map(program.structs.map((struct) => [struct.name, struct]));

  for (const probe of program.probes) {
    walkStatements(probe.body, (call) => {
      const dot = call.callee.lastIndexOf(".");
      if (dot <= 0) return;
      const mapName = call.callee.slice(0, dot);
      const method = call.callee.slice(dot + 1);
      if (method !== "emit") return;
      const map = maps.get(mapName);
      if (!map || map.kind !== "ringbuf" || map.valueType.kind !== "named") return;
      const struct = structs.get(map.valueType.name);
      if (!struct) return;
      validateRingbufEmit(map, struct, call);
    });
  }
}
