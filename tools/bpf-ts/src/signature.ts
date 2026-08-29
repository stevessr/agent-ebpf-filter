import * as ts from "typescript";
import { BpfTsCompileError } from "./diagnostics";

function fail(source: ts.SourceFile, node: ts.Node, message: string): never {
  const pos = source.getLineAndCharacterOfPosition(node.getStart(source));
  throw new BpfTsCompileError(message, source.fileName, pos.line + 1, pos.character + 1);
}

function hasAttachDecorator(node: ts.MethodDeclaration): boolean {
  const decorators = ts.canHaveDecorators(node) ? ts.getDecorators(node) ?? [] : [];
  return decorators.some((decorator) => {
    const expr = decorator.expression;
    return ts.isCallExpression(expr) &&
      ts.isIdentifier(expr.expression) &&
      ["kprobe", "uprobe", "tracepoint"].includes(expr.expression.text);
  });
}

export function validateProbeSignatures(sourceText: string, fileName: string) {
  const source = ts.createSourceFile(fileName, sourceText, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  for (const statement of source.statements) {
    if (!ts.isClassDeclaration(statement)) continue;
    for (const member of statement.members) {
      if (!ts.isMethodDeclaration(member) || !hasAttachDecorator(member)) continue;
      if (!member.type) {
        fail(source, member, "probe methods require an explicit ': i32' return type");
      }
      if (member.type.getText(source) !== "i32") {
        fail(source, member.type, `probe methods must return i32, not '${member.type.getText(source)}'`);
      }
      if (member.parameters.length !== 1) continue;
      const parameter = member.parameters[0];
      if (!parameter.type) {
        fail(source, parameter, "probe context parameters require an explicit context type");
      }
      const contextType = parameter.type.getText(source);
      if (!["ProbeContext", "KProbeContext", "UProbeContext", "TracepointContext"].includes(contextType)) {
        fail(source, parameter.type, `unsupported probe context type '${contextType}'`);
      }
    }
  }
}
