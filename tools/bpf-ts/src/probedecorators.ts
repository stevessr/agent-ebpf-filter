import * as ts from "typescript";
import { BpfTsCompileError } from "./diagnostics";
import type { ProbeKind, ProgramIR } from "./ir";

type ReturnProbeKind = Extract<ProbeKind, "kretprobe" | "uretprobe">;

export interface NormalizedProbeSource {
  sourceText: string;
  returnKinds: Map<string, ReturnProbeKind>;
}

function collectReturnKinds(source: ts.SourceFile): Map<string, ReturnProbeKind> {
  const kinds = new Map<string, ReturnProbeKind>();
  for (const statement of source.statements) {
    if (!ts.isClassDeclaration(statement)) continue;
    for (const member of statement.members) {
      if (!ts.isMethodDeclaration(member) || !member.name || !ts.isIdentifier(member.name)) continue;
      const decorators = ts.canHaveDecorators(member) ? ts.getDecorators(member) ?? [] : [];
      for (const decorator of decorators) {
        const expression = decorator.expression;
        if (!ts.isCallExpression(expression) || !ts.isIdentifier(expression.expression)) continue;
        const name = expression.expression.text;
        if (name !== "kretprobe" && name !== "uretprobe") continue;
        if (kinds.has(member.name.text)) {
          throw new BpfTsCompileError(`probe '${member.name.text}' has multiple return-probe decorators`, source.fileName);
        }
        kinds.set(member.name.text, name);
      }
    }
  }
  return kinds;
}

export function normalizeReturnProbeDecorators(sourceText: string, fileName: string): NormalizedProbeSource {
  const source = ts.createSourceFile(fileName, sourceText, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  const returnKinds = collectReturnKinds(source);
  if (returnKinds.size === 0) return { sourceText, returnKinds };

  const transformer: ts.TransformerFactory<ts.SourceFile> = (context) => {
    const visit: ts.Visitor = (node) => {
      if (ts.isDecorator(node)) {
        const expression = node.expression;
        if (ts.isCallExpression(expression) && ts.isIdentifier(expression.expression)) {
          const name = expression.expression.text;
          if (name === "kretprobe" || name === "uretprobe") {
            const replacement = name === "kretprobe" ? "kprobe" : "uprobe";
            return ts.factory.createDecorator(
              ts.factory.updateCallExpression(
                expression,
                ts.factory.createIdentifier(replacement),
                expression.typeArguments,
                expression.arguments,
              ),
            );
          }
        }
      }
      return ts.visitEachChild(node, visit, context);
    };
    return (root) => ts.visitNode(root, visit) as ts.SourceFile;
  };

  const transformed = ts.transform(source, [transformer]);
  try {
    const printer = ts.createPrinter({ newLine: ts.NewLineKind.LineFeed });
    return {
      sourceText: printer.printFile(transformed.transformed[0]),
      returnKinds,
    };
  } finally {
    transformed.dispose();
  }
}

export function applyReturnProbeKinds(program: ProgramIR, returnKinds: Map<string, ReturnProbeKind>) {
  for (const probe of program.probes) {
    const kind = returnKinds.get(probe.name);
    if (kind) probe.attach.kind = kind;
  }
}
