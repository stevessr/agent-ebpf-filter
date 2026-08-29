import { BpfTsCompileError } from "./diagnostics";
import type { ExprIR, ProgramIR, StmtIR } from "./ir";

function isDirectLookup(expr: ExprIR) {
  return expr.kind === "call" && (expr.callee.endsWith(".getOr") || expr.callee.endsWith(".takeOr"));
}

function rejectNestedLookup(expr: ExprIR) {
  if (isDirectLookup(expr)) {
    throw new BpfTsCompileError(
      `${(expr as Extract<ExprIR, { kind: "call" }>).callee}() must be used as a direct local initializer`,
    );
  }
  switch (expr.kind) {
    case "property":
      rejectNestedLookup(expr.object);
      break;
    case "binary":
      rejectNestedLookup(expr.left);
      rejectNestedLookup(expr.right);
      break;
    case "unary":
      rejectNestedLookup(expr.value);
      break;
    case "call":
      for (const arg of expr.args) rejectNestedLookup(arg);
      break;
    case "object":
      for (const field of expr.fields) rejectNestedLookup(field.value);
      break;
    default:
      break;
  }
}

function validateStatements(statements: StmtIR[]) {
  for (const statement of statements) {
    switch (statement.kind) {
      case "let":
        if (isDirectLookup(statement.value)) {
          const call = statement.value as Extract<ExprIR, { kind: "call" }>;
          for (const arg of call.args) rejectNestedLookup(arg);
        } else {
          rejectNestedLookup(statement.value);
        }
        break;
      case "assign":
        rejectNestedLookup(statement.target);
        rejectNestedLookup(statement.value);
        break;
      case "expr":
      case "return":
        rejectNestedLookup(statement.value);
        break;
      case "if":
        rejectNestedLookup(statement.test);
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
