import { BpfTsCompileError } from "./diagnostics";
import type { ExprIR, ProgramIR, StmtIR } from "./ir";

function isGetOr(expr: ExprIR) {
  return expr.kind === "call" && expr.callee.endsWith(".getOr");
}

function rejectNestedGetOr(expr: ExprIR) {
  if (isGetOr(expr)) {
    throw new BpfTsCompileError(
      `${(expr as Extract<ExprIR, { kind: "call" }>).callee}() must be used as a direct local initializer`,
    );
  }
  switch (expr.kind) {
    case "property":
      rejectNestedGetOr(expr.object);
      break;
    case "binary":
      rejectNestedGetOr(expr.left);
      rejectNestedGetOr(expr.right);
      break;
    case "unary":
      rejectNestedGetOr(expr.value);
      break;
    case "call":
      for (const arg of expr.args) rejectNestedGetOr(arg);
      break;
    case "object":
      for (const field of expr.fields) rejectNestedGetOr(field.value);
      break;
    default:
      break;
  }
}

function validateStatements(statements: StmtIR[]) {
  for (const statement of statements) {
    switch (statement.kind) {
      case "let":
        if (isGetOr(statement.value)) {
          const call = statement.value as Extract<ExprIR, { kind: "call" }>;
          for (const arg of call.args) rejectNestedGetOr(arg);
        } else {
          rejectNestedGetOr(statement.value);
        }
        break;
      case "assign":
        rejectNestedGetOr(statement.target);
        rejectNestedGetOr(statement.value);
        break;
      case "expr":
      case "return":
        rejectNestedGetOr(statement.value);
        break;
      case "if":
        rejectNestedGetOr(statement.test);
        validateStatements(statement.then);
        validateStatements(statement.otherwise);
        break;
      case "for":
        validateStatements(statement.body);
        break;
    }
  }
}

export function validateMapExpressions(program: ProgramIR) {
  for (const probe of program.probes) validateStatements(probe.body);
}
