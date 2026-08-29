import { BpfTsCompileError } from "./diagnostics";
import type { ExprIR, ProgramIR, StmtIR } from "./ir";

function isArgI32(expr: ExprIR): boolean {
  return expr.kind === "call" && expr.callee === "bpf.argI32";
}

function rejectNestedArgI32(expr: ExprIR): void {
  if (isArgI32(expr)) {
    throw new BpfTsCompileError("bpf.argI32() must be used as a direct local initializer");
  }
  switch (expr.kind) {
    case "property":
      rejectNestedArgI32(expr.object);
      return;
    case "binary":
      rejectNestedArgI32(expr.left);
      rejectNestedArgI32(expr.right);
      return;
    case "unary":
      rejectNestedArgI32(expr.value);
      return;
    case "call":
      for (const arg of expr.args) rejectNestedArgI32(arg);
      return;
    case "object":
      for (const field of expr.fields) rejectNestedArgI32(field.value);
      return;
    default:
      return;
  }
}

function lowerStatements(statements: StmtIR[]): void {
  for (const statement of statements) {
    switch (statement.kind) {
      case "let":
        if (statement.value.kind === "call" && statement.value.callee === "bpf.argI32") {
          if (statement.type?.kind !== "scalar" || statement.type.name !== "i32") {
            throw new BpfTsCompileError(
              `bpf.argI32() local '${statement.name}' must infer to i32 before lowering`,
            );
          }
          for (const arg of statement.value.args) rejectNestedArgI32(arg);
          statement.value.callee = "bpf.arg";
        } else {
          rejectNestedArgI32(statement.value);
        }
        break;
      case "assign":
        rejectNestedArgI32(statement.target);
        rejectNestedArgI32(statement.value);
        break;
      case "expr":
      case "return":
        rejectNestedArgI32(statement.value);
        break;
      case "if":
        rejectNestedArgI32(statement.test);
        lowerStatements(statement.then);
        lowerStatements(statement.otherwise);
        break;
      case "for":
        lowerStatements(statement.body);
        break;
    }
  }
}

// argI32 is deliberately lowered after type inference. The local retains its
// inferred __s32 C type while the backend reuses the existing PT_REGS_PARMn
// lowering, so the C assignment performs the ABI-correct 32-bit truncation and
// signed interpretation without introducing another backend expression form.
export function lowerArgI32(program: ProgramIR): void {
  for (const probe of program.probes) lowerStatements(probe.body);
}
