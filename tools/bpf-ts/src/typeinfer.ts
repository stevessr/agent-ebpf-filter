import { BpfTsCompileError } from "./diagnostics";
import type { BpfType, ExprIR, ProgramIR, ScalarType, StmtIR } from "./ir";

type ScalarBpfType = Extract<BpfType, { kind: "scalar" }>;
type TypeEnv = Map<string, BpfType>;

const comparisonOps = new Set(["<", "<=", ">", ">=", "===", "!==", "&&", "||"]);

function scalar(name: ScalarType): ScalarBpfType {
  return { kind: "scalar", name };
}

function coreFieldType(program: ProgramIR, callee: string): BpfType | null {
  if (!callee.startsWith("bpf.coreRead.")) return null;
  const parts = callee.split(".");
  if (parts.length !== 4) return null;
  const struct = program.structs.find((candidate) => candidate.name === parts[2]);
  const field = struct?.fields.find((candidate) => candidate.name === parts[3]);
  return field?.type ?? null;
}

function mapLookupType(program: ProgramIR, callee: string): BpfType | null {
  const dot = callee.lastIndexOf(".");
  if (dot <= 0) return null;
  const method = callee.slice(dot + 1);
  if (method !== "getOr" && method !== "takeOr") return null;
  const map = program.maps.find((candidate) => candidate.name === callee.slice(0, dot));
  return map?.valueType ?? null;
}

function inferExpr(program: ProgramIR, env: TypeEnv, expr: ExprIR): BpfType {
  switch (expr.kind) {
    case "number":
      return scalar("u64");
    case "boolean":
      return scalar("bool");
    case "identifier": {
      const type = env.get(expr.name);
      if (!type) throw new BpfTsCompileError(`cannot infer type of unknown identifier '${expr.name}'`);
      return type;
    }
    case "property":
      throw new BpfTsCompileError("property expression type inference is not supported");
    case "unary":
      if (expr.op === "!") return scalar("bool");
      return inferExpr(program, env, expr.value);
    case "binary":
      if (comparisonOps.has(expr.op)) return scalar("bool");
      return mergeNumericTypes(
        inferExpr(program, env, expr.left),
        inferExpr(program, env, expr.right),
      );
    case "call": {
      if (["bpf.pid", "bpf.tid", "bpf.uid", "bpf.gid"].includes(expr.callee)) return scalar("u32");
      if (["bpf.ktimeNs", "bpf.arg", "bpf.currentTask"].includes(expr.callee)) return scalar("u64");
      if (expr.callee === "bpf.argI32") return scalar("i32");
      if (expr.callee === "bpf.ret") return scalar("i64");
      if (expr.callee === "bpf.retI32") return scalar("i32");
      if (expr.callee === "bpf.comm") return { kind: "bytes", length: 16 };
      if (expr.callee === "bpf.userString") return { kind: "bytes", length: 256 };
      if (expr.callee === "bpf.userBytes") return { kind: "bytes", length: 4096 };
      const lookupType = mapLookupType(program, expr.callee);
      if (lookupType) return lookupType;
      const coreType = coreFieldType(program, expr.callee);
      if (coreType) return coreType;
      throw new BpfTsCompileError(`call '${expr.callee}' has no local expression type`);
    }
    case "object":
      throw new BpfTsCompileError("object literals do not have a local scalar type");
  }
}

function integerRank(type: ScalarBpfType): number {
  switch (type.name) {
    case "bool":
    case "u8":
    case "i8":
      return 1;
    case "u16":
    case "i16":
      return 2;
    case "u32":
    case "i32":
      return 3;
    case "u64":
    case "i64":
      return 4;
  }
}

function isSigned(type: ScalarBpfType): boolean {
  return type.name.startsWith("i");
}

function mergeNumericTypes(left: BpfType, right: BpfType): ScalarBpfType {
  if (left.kind !== "scalar" || right.kind !== "scalar") {
    throw new BpfTsCompileError("arithmetic expressions require scalar operands");
  }
  const leftRank = integerRank(left);
  const rightRank = integerRank(right);
  const rank = Math.max(leftRank, rightRank);
  const signed = isSigned(left) && isSigned(right);
  if (rank <= 1) return scalar(signed ? "i8" : "u8");
  if (rank === 2) return scalar(signed ? "i16" : "u16");
  if (rank === 3) return scalar(signed ? "i32" : "u32");
  return scalar(signed ? "i64" : "u64");
}

function inferStatements(program: ProgramIR, statements: StmtIR[], env: TypeEnv) {
  for (const statement of statements) {
    switch (statement.kind) {
      case "let": {
        if (!statement.type) statement.type = inferExpr(program, env, statement.value);
        env.set(statement.name, statement.type);
        break;
      }
      case "assign":
        inferExpr(program, env, statement.value);
        break;
      case "expr":
        break;
      case "return":
        inferExpr(program, env, statement.value);
        break;
      case "if":
        inferExpr(program, env, statement.test);
        inferStatements(program, statement.then, new Map(env));
        inferStatements(program, statement.otherwise, new Map(env));
        break;
      case "for": {
        const loopEnv = new Map(env);
        loopEnv.set(statement.variable, scalar("i32"));
        inferStatements(program, statement.body, loopEnv);
        break;
      }
    }
  }
}

export function inferLocalTypes(program: ProgramIR) {
  for (const probe of program.probes) {
    const env: TypeEnv = new Map([[probe.contextName, scalar("u64")]]);
    inferStatements(program, probe.body, env);
  }
}
