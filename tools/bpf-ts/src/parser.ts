import * as ts from "typescript";
import { BpfTsCompileError } from "./diagnostics";
import type {
  BpfType,
  ExprIR,
  MapIR,
  ProbeAttachIR,
  ProbeIR,
  ProgramIR,
  ScalarType,
  StmtIR,
  StructIR,
} from "./ir";

const scalarTypes = new Set<ScalarType>([
  "u8",
  "u16",
  "u32",
  "u64",
  "i8",
  "i16",
  "i32",
  "i64",
  "bool",
]);
const maxBoundedLoopIterations = 64;

type ProbeNode = ts.FunctionDeclaration | ts.MethodDeclaration;

function fail(source: ts.SourceFile, node: ts.Node, message: string): never {
  const pos = source.getLineAndCharacterOfPosition(node.getStart(source));
  throw new BpfTsCompileError(message, source.fileName, pos.line + 1, pos.character + 1);
}

function parseIntegerConstant(source: ts.SourceFile, node: ts.Expression): number {
  if (ts.isParenthesizedExpression(node)) return parseIntegerConstant(source, node.expression);
  if (ts.isNumericLiteral(node)) {
    const value = Number(node.text);
    if (Number.isSafeInteger(value)) return value;
  }
  if (ts.isPrefixUnaryExpression(node) && node.operator === ts.SyntaxKind.MinusToken) {
    return -parseIntegerConstant(source, node.operand);
  }
  if (ts.isBinaryExpression(node)) {
    const left = parseIntegerConstant(source, node.left);
    const right = parseIntegerConstant(source, node.right);
    switch (node.operatorToken.kind) {
      case ts.SyntaxKind.PlusToken:
        return left + right;
      case ts.SyntaxKind.MinusToken:
        return left - right;
      case ts.SyntaxKind.AsteriskToken:
        return left * right;
      case ts.SyntaxKind.LessThanLessThanToken:
        return left << right;
      case ts.SyntaxKind.GreaterThanGreaterThanToken:
        return left >> right;
      default:
        break;
    }
  }
  return fail(source, node, "expected a compile-time integer constant");
}

function parseType(source: ts.SourceFile, node: ts.TypeNode): BpfType {
  if (node.kind === ts.SyntaxKind.AnyKeyword) return fail(source, node, "'any' is not allowed in bpf-ts");
  if (!ts.isTypeReferenceNode(node)) return fail(source, node, `unsupported BPF type '${node.getText(source)}'`);
  const name = node.typeName.getText(source);
  if (scalarTypes.has(name as ScalarType)) return { kind: "scalar", name: name as ScalarType };
  if (name === "bytes") {
    const arg = node.typeArguments?.[0];
    if (!arg || !ts.isLiteralTypeNode(arg) || !ts.isNumericLiteral(arg.literal)) {
      return fail(source, node, "bytes<N> requires a positive numeric literal length");
    }
    const length = Number(arg.literal.text);
    if (!Number.isInteger(length) || length <= 0 || length > 4096) {
      return fail(source, node, "bytes<N> length must be between 1 and 4096");
    }
    return { kind: "bytes", length };
  }
  return { kind: "named", name };
}

function parseStruct(source: ts.SourceFile, node: ts.InterfaceDeclaration): StructIR {
  if (node.typeParameters?.length || node.heritageClauses?.length) {
    return fail(source, node, "BPF structs cannot use generics or interface inheritance");
  }
  const fields = node.members.map((member) => {
    if (!ts.isPropertySignature(member) || !member.type || !member.name || !ts.isIdentifier(member.name)) {
      return fail(source, member, "BPF struct fields must be named, required properties with explicit types");
    }
    if (member.questionToken) return fail(source, member, "optional BPF struct fields are not supported");
    return { name: member.name.text, type: parseType(source, member.type) };
  });
  return { name: node.name.text, fields };
}

function parseMap(source: ts.SourceFile, decl: ts.VariableDeclaration): MapIR | null {
  if (!ts.isIdentifier(decl.name) || !decl.initializer || !ts.isCallExpression(decl.initializer)) return null;
  const call = decl.initializer;
  if (!ts.isIdentifier(call.expression)) return null;
  const kind = call.expression.text;
  if (kind !== "ringbuf" && kind !== "hash" && kind !== "array") return null;
  if (call.arguments.length !== 1) return fail(source, call, `${kind}() requires exactly one capacity argument`);
  const maxEntries = parseIntegerConstant(source, call.arguments[0]);
  if (maxEntries <= 0 || maxEntries > 1 << 30) return fail(source, call, "map capacity is outside the supported range");
  const args = call.typeArguments ?? [];
  if (kind === "ringbuf") {
    if (args.length !== 1) return fail(source, call, "ringbuf<Value>(bytes) requires one value type");
    return { name: decl.name.text, kind, valueType: parseType(source, args[0]), maxEntries };
  }
  if (kind === "hash") {
    if (args.length !== 2) return fail(source, call, "hash<Key, Value>(maxEntries) requires key and value types");
    return {
      name: decl.name.text,
      kind,
      keyType: parseType(source, args[0]),
      valueType: parseType(source, args[1]),
      maxEntries,
    };
  }
  if (args.length !== 1) return fail(source, call, "array<Value>(maxEntries) requires one value type");
  return {
    name: decl.name.text,
    kind,
    keyType: { kind: "scalar", name: "u32" },
    valueType: parseType(source, args[0]),
    maxEntries,
  };
}

function decoratorCall(source: ts.SourceFile, node: ProbeNode): ProbeAttachIR | null {
  const decorators = ts.canHaveDecorators(node) ? ts.getDecorators(node) ?? [] : [];
  if (decorators.length === 0) return null;
  if (decorators.length !== 1) return fail(source, node, "probe methods require exactly one attach decorator");
  const expr = decorators[0].expression;
  if (!ts.isCallExpression(expr) || !ts.isIdentifier(expr.expression)) {
    return fail(source, expr, "probe decorator must be a direct call such as @kprobe(\"do_sys_open\")");
  }
  const name = expr.expression.text;
  const stringArg = (index: number) => {
    const arg = expr.arguments[index];
    if (!arg || !ts.isStringLiteralLike(arg)) return fail(source, expr, `@${name} requires string literal arguments`);
    return arg.text;
  };
  if (name === "kprobe") {
    if (expr.arguments.length !== 1) return fail(source, expr, "@kprobe requires one kernel symbol");
    return { kind: "kprobe", target: stringArg(0) };
  }
  if (name === "uprobe") {
    if (expr.arguments.length !== 1) return fail(source, expr, "@uprobe requires one userspace symbol");
    return { kind: "uprobe", target: stringArg(0) };
  }
  if (name === "tracepoint") {
    if (expr.arguments.length !== 2) return fail(source, expr, "@tracepoint requires category and event");
    return { kind: "tracepoint", category: stringArg(0), event: stringArg(1) };
  }
  return fail(source, expr, `unsupported probe decorator @${name}`);
}

function calleeName(source: ts.SourceFile, node: ts.Expression): string {
  if (ts.isIdentifier(node)) return node.text;
  if (ts.isPropertyAccessExpression(node)) return `${calleeName(source, node.expression)}.${node.name.text}`;
  return fail(source, node, "only direct and dotted function calls are supported");
}

function parseExpr(source: ts.SourceFile, node: ts.Expression): ExprIR {
  if (ts.isParenthesizedExpression(node)) return parseExpr(source, node.expression);
  if (ts.isAsExpression(node) || ts.isTypeAssertionExpression(node)) return parseExpr(source, node.expression);
  if (ts.isNumericLiteral(node)) return { kind: "number", value: Number(node.text) };
  if (node.kind === ts.SyntaxKind.TrueKeyword) return { kind: "boolean", value: true };
  if (node.kind === ts.SyntaxKind.FalseKeyword) return { kind: "boolean", value: false };
  if (ts.isIdentifier(node)) return { kind: "identifier", name: node.text };
  if (ts.isPropertyAccessExpression(node)) {
    return { kind: "property", object: parseExpr(source, node.expression), property: node.name.text };
  }
  if (ts.isPrefixUnaryExpression(node)) {
    const allowed = new Set([ts.SyntaxKind.ExclamationToken, ts.SyntaxKind.MinusToken, ts.SyntaxKind.TildeToken]);
    if (!allowed.has(node.operator)) return fail(source, node, "unsupported unary operator");
    return { kind: "unary", op: ts.tokenToString(node.operator) ?? "", value: parseExpr(source, node.operand) };
  }
  if (ts.isBinaryExpression(node)) {
    const allowed = new Set([
      ts.SyntaxKind.PlusToken,
      ts.SyntaxKind.MinusToken,
      ts.SyntaxKind.AsteriskToken,
      ts.SyntaxKind.SlashToken,
      ts.SyntaxKind.PercentToken,
      ts.SyntaxKind.AmpersandToken,
      ts.SyntaxKind.BarToken,
      ts.SyntaxKind.CaretToken,
      ts.SyntaxKind.LessThanLessThanToken,
      ts.SyntaxKind.GreaterThanGreaterThanToken,
      ts.SyntaxKind.LessThanToken,
      ts.SyntaxKind.LessThanEqualsToken,
      ts.SyntaxKind.GreaterThanToken,
      ts.SyntaxKind.GreaterThanEqualsToken,
      ts.SyntaxKind.EqualsEqualsEqualsToken,
      ts.SyntaxKind.ExclamationEqualsEqualsToken,
      ts.SyntaxKind.AmpersandAmpersandToken,
      ts.SyntaxKind.BarBarToken,
    ]);
    if (!allowed.has(node.operatorToken.kind)) return fail(source, node, "unsupported binary operator");
    return {
      kind: "binary",
      op: node.operatorToken.getText(source),
      left: parseExpr(source, node.left),
      right: parseExpr(source, node.right),
    };
  }
  if (ts.isCallExpression(node)) {
    return { kind: "call", callee: calleeName(source, node.expression), args: node.arguments.map((arg) => parseExpr(source, arg)) };
  }
  if (ts.isObjectLiteralExpression(node)) {
    const fields = node.properties.map((property) => {
      if (ts.isShorthandPropertyAssignment(property)) {
        return { name: property.name.text, value: { kind: "identifier", name: property.name.text } as ExprIR };
      }
      if (ts.isPropertyAssignment(property) && (ts.isIdentifier(property.name) || ts.isStringLiteralLike(property.name))) {
        return { name: property.name.text, value: parseExpr(source, property.initializer) };
      }
      return fail(source, property, "BPF object literals support only static property assignments");
    });
    return { kind: "object", fields };
  }
  return fail(source, node, `unsupported expression '${node.getText(source)}'`);
}

function parseBlock(source: ts.SourceFile, block: ts.Block, probeName: string): StmtIR[] {
  return block.statements.flatMap((statement) => parseStatement(source, statement, probeName));
}

function parseFor(source: ts.SourceFile, node: ts.ForStatement, probeName: string): StmtIR {
  if (!node.initializer || !ts.isVariableDeclarationList(node.initializer) || node.initializer.declarations.length !== 1) {
    return fail(source, node, "bounded for loops require 'for (let i = CONST; ...)' syntax");
  }
  const decl = node.initializer.declarations[0];
  if (!ts.isIdentifier(decl.name) || !decl.initializer) return fail(source, decl, "loop variable must be a named constant initializer");
  const variable = decl.name.text;
  const start = parseIntegerConstant(source, decl.initializer);
  if (!node.condition || !ts.isBinaryExpression(node.condition) || !ts.isIdentifier(node.condition.left) || node.condition.left.text !== variable) {
    return fail(source, node, "loop condition must compare the loop variable against a constant bound");
  }
  const bound = parseIntegerConstant(source, node.condition.right);
  let endExclusive: number;
  if (node.condition.operatorToken.kind === ts.SyntaxKind.LessThanToken) endExclusive = bound;
  else if (node.condition.operatorToken.kind === ts.SyntaxKind.LessThanEqualsToken) endExclusive = bound + 1;
  else return fail(source, node.condition, "bounded loops support only '<' or '<=' conditions");

  let step = 1;
  if (!node.incrementor) return fail(source, node, "bounded loop requires a constant positive increment");
  if (ts.isPostfixUnaryExpression(node.incrementor) || ts.isPrefixUnaryExpression(node.incrementor)) {
    if (node.incrementor.operator !== ts.SyntaxKind.PlusPlusToken || !ts.isIdentifier(node.incrementor.operand) || node.incrementor.operand.text !== variable) {
      return fail(source, node.incrementor, "bounded loops support only ++ or += CONST increments");
    }
  } else if (
    ts.isBinaryExpression(node.incrementor) &&
    node.incrementor.operatorToken.kind === ts.SyntaxKind.PlusEqualsToken &&
    ts.isIdentifier(node.incrementor.left) &&
    node.incrementor.left.text === variable
  ) {
    step = parseIntegerConstant(source, node.incrementor.right);
  } else {
    return fail(source, node.incrementor, "bounded loops support only ++ or += CONST increments");
  }
  if (step <= 0) return fail(source, node.incrementor, "loop increment must be positive");
  const iterations = Math.max(0, Math.ceil((endExclusive - start) / step));
  if (iterations > maxBoundedLoopIterations) {
    return fail(source, node, `loop has ${iterations} iterations; maximum is ${maxBoundedLoopIterations}`);
  }
  const body = ts.isBlock(node.statement)
    ? parseBlock(source, node.statement, probeName)
    : parseStatement(source, node.statement, probeName);
  return { kind: "for", variable, start, endExclusive, step, body };
}

function parseStatement(source: ts.SourceFile, node: ts.Statement, probeName: string): StmtIR[] {
  if (ts.isVariableStatement(node)) {
    return node.declarationList.declarations.map((decl) => {
      if (!ts.isIdentifier(decl.name) || !decl.initializer) return fail(source, decl, "local variables require a simple name and initializer");
      return {
        kind: "let" as const,
        name: decl.name.text,
        type: decl.type ? parseType(source, decl.type) : undefined,
        value: parseExpr(source, decl.initializer),
      };
    });
  }
  if (ts.isReturnStatement(node)) {
    if (!node.expression) return fail(source, node, "probe functions must return an explicit integer value");
    return [{ kind: "return", value: parseExpr(source, node.expression) }];
  }
  if (ts.isExpressionStatement(node)) {
    const expr = node.expression;
    if (ts.isBinaryExpression(expr)) {
      const assignmentOps = new Map<ts.SyntaxKind, string>([
        [ts.SyntaxKind.EqualsToken, "="],
        [ts.SyntaxKind.PlusEqualsToken, "+="],
        [ts.SyntaxKind.MinusEqualsToken, "-="],
        [ts.SyntaxKind.AmpersandEqualsToken, "&="],
        [ts.SyntaxKind.BarEqualsToken, "|="],
      ]);
      const op = assignmentOps.get(expr.operatorToken.kind);
      if (op) return [{ kind: "assign", target: parseExpr(source, expr.left), value: parseExpr(source, expr.right), op }];
    }
    const parsed = parseExpr(source, expr);
    if (parsed.kind === "call" && parsed.callee === probeName) return fail(source, node, "recursive probe calls are not allowed");
    return [{ kind: "expr", value: parsed }];
  }
  if (ts.isIfStatement(node)) {
    const block = (statement: ts.Statement) =>
      ts.isBlock(statement) ? parseBlock(source, statement, probeName) : parseStatement(source, statement, probeName);
    return [{
      kind: "if",
      test: parseExpr(source, node.expression),
      then: block(node.thenStatement),
      otherwise: node.elseStatement ? block(node.elseStatement) : [],
    }];
  }
  if (ts.isForStatement(node)) return [parseFor(source, node, probeName)];
  return fail(source, node, `statement '${ts.SyntaxKind[node.kind]}' is not verifier-safe in bpf-ts`);
}

function probeNodeName(source: ts.SourceFile, node: ProbeNode): string {
  if (!node.name || !ts.isIdentifier(node.name)) return fail(source, node, "probe method requires an identifier name");
  return node.name.text;
}

function parseProbe(source: ts.SourceFile, node: ProbeNode): ProbeIR | null {
  const attach = decoratorCall(source, node);
  if (!attach) return null;
  if (!node.body) return fail(source, node, "probe method requires a body");
  if (node.modifiers?.some((m) => m.kind === ts.SyntaxKind.AsyncKeyword) || node.asteriskToken) {
    return fail(source, node, "async and generator probe methods are not supported");
  }
  if (ts.isMethodDeclaration(node)) {
    if (!node.modifiers?.some((m) => m.kind === ts.SyntaxKind.StaticKeyword)) {
      return fail(source, node, "probe namespace methods must be static");
    }
    if (node.questionToken) return fail(source, node, "optional probe methods are not supported");
  }
  if (node.parameters.length !== 1) return fail(source, node, "probe methods require exactly one context parameter");
  const parameter = node.parameters[0];
  if (!ts.isIdentifier(parameter.name)) return fail(source, parameter, "probe context parameter must be an identifier");
  const name = probeNodeName(source, node);
  const contextType = parameter.type?.getText(source) ?? "ProbeContext";
  return {
    name,
    attach,
    contextName: parameter.name.text,
    contextType,
    body: parseBlock(source, node.body, name),
  };
}

function parseProbeClass(source: ts.SourceFile, node: ts.ClassDeclaration): ProbeIR[] {
  if (!node.name) return fail(source, node, "probe namespace class requires a name");
  if (node.typeParameters?.length || node.heritageClauses?.length) {
    return fail(source, node, "probe namespace classes cannot use generics or inheritance");
  }
  const probes: ProbeIR[] = [];
  for (const member of node.members) {
    if (!ts.isMethodDeclaration(member)) {
      return fail(source, member, "probe namespace classes may contain only decorated static methods");
    }
    const probe = parseProbe(source, member);
    if (!probe) return fail(source, member, "probe namespace methods require an attach decorator");
    probes.push(probe);
  }
  return probes;
}

export function parseBpfTs(sourceText: string, fileName = "program.ts"): ProgramIR {
  const source = ts.createSourceFile(fileName, sourceText, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  const parseDiagnostics = (source as ts.SourceFile & { parseDiagnostics?: readonly ts.Diagnostic[] }).parseDiagnostics ?? [];
  if (parseDiagnostics.length > 0) {
    const diagnostic = parseDiagnostics[0];
    const message = ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n");
    const position = diagnostic.start ?? 0;
    const pos = source.getLineAndCharacterOfPosition(position);
    throw new BpfTsCompileError(`TypeScript syntax error: ${message}`, fileName, pos.line + 1, pos.character + 1);
  }

  const structs: StructIR[] = [];
  const maps: MapIR[] = [];
  const probes: ProbeIR[] = [];

  for (const statement of source.statements) {
    if (ts.isInterfaceDeclaration(statement)) {
      structs.push(parseStruct(source, statement));
      continue;
    }
    if (ts.isVariableStatement(statement)) {
      for (const decl of statement.declarationList.declarations) {
        const map = parseMap(source, decl);
        if (!map) return fail(source, decl, "top-level variables must declare ringbuf/hash/array maps");
        maps.push(map);
      }
      continue;
    }
    if (ts.isClassDeclaration(statement)) {
      probes.push(...parseProbeClass(source, statement));
      continue;
    }
    if (ts.isFunctionDeclaration(statement)) {
      return fail(source, statement, "top-level functions cannot be decorated in TypeScript; use a probe namespace class with decorated static methods");
    }
    if (ts.isTypeAliasDeclaration(statement) || ts.isEmptyStatement(statement)) continue;
    return fail(source, statement, `unsupported top-level declaration '${ts.SyntaxKind[statement.kind]}'`);
  }

  const names = new Set<string>();
  for (const probe of probes) {
    if (names.has(probe.name)) throw new BpfTsCompileError(`duplicate probe name '${probe.name}'`, fileName);
    names.add(probe.name);
  }
  if (probes.length === 0) throw new BpfTsCompileError("program contains no probe methods", fileName);
  return { structs, maps, probes };
}
