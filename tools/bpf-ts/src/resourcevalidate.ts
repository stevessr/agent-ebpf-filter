import { BpfTsCompileError } from "./diagnostics";
import type { BpfType, ProgramIR, StmtIR } from "./ir";

// The kernel BPF stack limit is 512 bytes on the supported targets. Keep a
// conservative reserve for clang-generated temporaries, helper arguments and
// CO-RE macro scratch state instead of allowing the DSL to consume all 512.
const maxDslLocalStackBytes = 384;

// Bounded loops are unrolled by the C backend. This policy keeps nested loops
// from turning a small TypeScript source file into pathological verifier input.
const maxEstimatedExpandedStatements = 4096;

function scalarBytes(type: BpfType): number {
  if (type.kind !== "scalar") {
    throw new BpfTsCompileError("only scalar local variables are supported in the initial bpf-ts backend");
  }
  switch (type.name) {
    case "u8":
    case "i8":
    case "bool":
      return 1;
    case "u16":
    case "i16":
      return 2;
    case "u32":
    case "i32":
      return 4;
    case "u64":
    case "i64":
      return 8;
  }
}

function estimateLocalBytes(statements: StmtIR[]): number {
  let total = 0;
  for (const statement of statements) {
    switch (statement.kind) {
      case "let":
        if (statement.type) {
          total += scalarBytes(statement.type);
        } else {
          // Untyped locals are currently lowered to a scalar register-sized C
          // value by the backend. Count the worst scalar width conservatively.
          total += 8;
        }
        break;
      case "if":
        total += estimateLocalBytes(statement.then);
        total += estimateLocalBytes(statement.otherwise);
        break;
      case "for":
        total += 4; // C int loop induction variable.
        total += estimateLocalBytes(statement.body);
        break;
      default:
        break;
    }
  }
  return total;
}

function estimateExpandedStatements(statements: StmtIR[]): number {
  let total = 0;
  for (const statement of statements) {
    switch (statement.kind) {
      case "if":
        total += 1;
        total += estimateExpandedStatements(statement.then);
        total += estimateExpandedStatements(statement.otherwise);
        break;
      case "for": {
        const iterations = Math.max(
          0,
          Math.ceil((statement.endExclusive - statement.start) / statement.step),
        );
        total += 1 + iterations * estimateExpandedStatements(statement.body);
        break;
      }
      default:
        total += 1;
        break;
    }
    if (total > maxEstimatedExpandedStatements) return total;
  }
  return total;
}

export function validateVerifierResources(program: ProgramIR) {
  for (const probe of program.probes) {
    const localBytes = estimateLocalBytes(probe.body);
    if (localBytes > maxDslLocalStackBytes) {
      throw new BpfTsCompileError(
        `probe '${probe.name}' declares approximately ${localBytes} bytes of local storage; bpf-ts limits DSL locals to ${maxDslLocalStackBytes} bytes to preserve BPF stack headroom`,
      );
    }

    const expandedStatements = estimateExpandedStatements(probe.body);
    if (expandedStatements > maxEstimatedExpandedStatements) {
      throw new BpfTsCompileError(
        `probe '${probe.name}' expands to approximately ${expandedStatements} statements after loop unrolling; bpf-ts policy limit is ${maxEstimatedExpandedStatements}`,
      );
    }
  }
}
