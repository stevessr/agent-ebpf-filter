import { BpfTsCompileError } from "./diagnostics";
import type { ExprIR, ProgramIR, StmtIR } from "./ir";

function lowerStatements(statements: StmtIR[]): StmtIR[] {
  const lowered: StmtIR[] = [];
  for (const statement of statements) {
    if (
      statement.kind === "let" &&
      statement.value.kind === "call" &&
      statement.value.callee.endsWith(".takeOr")
    ) {
      const call = statement.value;
      if (call.args.length !== 2) {
        throw new BpfTsCompileError(`${call.callee}() expects 2 arguments`);
      }
      const key = call.args[0];
      if (key.kind !== "identifier") {
        throw new BpfTsCompileError(
          `${call.callee}() key must be a local identifier so lookup and delete use the exact same value`,
        );
      }
      const dot = call.callee.lastIndexOf(".");
      const mapName = call.callee.slice(0, dot);
      const getOr: ExprIR = {
        kind: "call",
        callee: `${mapName}.getOr`,
        args: call.args,
      };
      const remove: ExprIR = {
        kind: "call",
        callee: `${mapName}.delete`,
        args: [key],
      };
      lowered.push({ ...statement, value: getOr });
      lowered.push({ kind: "expr", value: remove });
      continue;
    }

    if (statement.kind === "if") {
      lowered.push({
        ...statement,
        then: lowerStatements(statement.then),
        otherwise: lowerStatements(statement.otherwise),
      });
      continue;
    }
    if (statement.kind === "for") {
      lowered.push({ ...statement, body: lowerStatements(statement.body) });
      continue;
    }
    lowered.push(statement);
  }
  return lowered;
}

export function lowerTakeOr(program: ProgramIR) {
  for (const probe of program.probes) {
    probe.body = lowerStatements(probe.body);
  }
}
