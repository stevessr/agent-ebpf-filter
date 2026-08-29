import * as ts from "typescript";
import { BpfTsCompileError } from "./diagnostics";
import type { ProgramIR } from "./ir";

function hasDeclareModifier(node: ts.InterfaceDeclaration): boolean {
  return node.modifiers?.some((modifier) => modifier.kind === ts.SyntaxKind.DeclareKeyword) ?? false;
}

export function markCoreTypeProjections(sourceText: string, fileName: string, program: ProgramIR) {
  const source = ts.createSourceFile(fileName, sourceText, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  const coreNames = new Set<string>();

  for (const statement of source.statements) {
    if (ts.isInterfaceDeclaration(statement) && hasDeclareModifier(statement)) {
      coreNames.add(statement.name.text);
    }
  }

  for (const struct of program.structs) {
    struct.core = coreNames.has(struct.name);
  }

  for (const map of program.maps) {
    if (map.valueType.kind === "named" && coreNames.has(map.valueType.name)) {
      throw new BpfTsCompileError(
        `kernel BTF projection '${map.valueType.name}' cannot be used directly as map '${map.name}' value; copy selected fields into a regular event interface`,
        fileName,
      );
    }
    if (map.keyType?.kind === "named" && coreNames.has(map.keyType.name)) {
      throw new BpfTsCompileError(
        `kernel BTF projection '${map.keyType.name}' cannot be used directly as map '${map.name}' key`,
        fileName,
      );
    }
  }

  for (const struct of program.structs) {
    if (struct.core) continue;
    for (const field of struct.fields) {
      if (field.type.kind === "named" && coreNames.has(field.type.name)) {
        throw new BpfTsCompileError(
          `event struct '${struct.name}' cannot embed kernel BTF projection '${field.type.name}' by value`,
          fileName,
        );
      }
    }
  }
}
