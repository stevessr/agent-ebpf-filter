import { BpfTsCompileError } from "./diagnostics";
import type { ExprIR, ProgramIR, StmtIR } from "./ir";

interface CoreAccess {
  structName: string;
  fieldName: string;
}

function parseCoreAccess(callee: string): CoreAccess | null {
  if (!callee.startsWith("bpf.coreRead.")) return null;
  const parts = callee.split(".");
  if (parts.length !== 4 || parts[0] !== "bpf" || parts[1] !== "coreRead") {
    throw new BpfTsCompileError(
      `invalid CO-RE helper '${callee}'; use bpf.coreRead.<Struct>.<field>(pointer)`,
    );
  }
  const structName = parts[2];
  const fieldName = parts[3];
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(structName) || !/^[A-Za-z_][A-Za-z0-9_]*$/.test(fieldName)) {
    throw new BpfTsCompileError(
      `invalid CO-RE helper '${callee}'; struct and field names must be C identifiers`,
    );
  }
  return { structName, fieldName };
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
  for (const statement of statements) {
    switch (statement.kind) {
      case "let":
        walkExpr(statement.value, visit);
        break;
      case "assign":
        walkExpr(statement.target, visit);
        walkExpr(statement.value, visit);
        break;
      case "expr":
      case "return":
        walkExpr(statement.value, visit);
        break;
      case "if":
        walkExpr(statement.test, visit);
        walkStatements(statement.then, visit);
        walkStatements(statement.otherwise, visit);
        break;
      case "for":
        walkStatements(statement.body, visit);
        break;
    }
  }
}

export function validateCoreAccesses(program: ProgramIR) {
  const structs = new Map(program.structs.map((struct) => [struct.name, struct]));

  for (const probe of program.probes) {
    walkStatements(probe.body, (expr) => {
      if (expr.kind !== "call") return;
      const access = parseCoreAccess(expr.callee);
      if (!access) return;
      if (expr.args.length !== 1) {
        throw new BpfTsCompileError(`${expr.callee}() expects exactly one kernel pointer argument`);
      }
      const struct = structs.get(access.structName);
      if (!struct) {
        throw new BpfTsCompileError(
          `CO-RE read in probe '${probe.name}' references unknown kernel struct '${access.structName}'`,
        );
      }
      if (!struct.core) {
        throw new BpfTsCompileError(
          `CO-RE struct '${access.structName}' must be declared as 'declare interface ${access.structName} { ... }'`,
        );
      }
      const field = struct.fields.find((candidate) => candidate.name === access.fieldName);
      if (!field) {
        throw new BpfTsCompileError(
          `CO-RE read references unknown field '${access.structName}.${access.fieldName}'`,
        );
      }
      if (field.type.kind !== "scalar") {
        throw new BpfTsCompileError(
          `CO-RE field '${access.structName}.${access.fieldName}' must be scalar in the initial backend`,
        );
      }
    });
  }
}
