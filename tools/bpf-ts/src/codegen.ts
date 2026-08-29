import { BpfTsCompileError } from "./diagnostics";
import type { BpfType, ExprIR, MapIR, ProgramIR, StmtIR } from "./ir";

const scalarCType: Record<string, string> = {
  u8: "__u8",
  u16: "__u16",
  u32: "__u32",
  u64: "__u64",
  i8: "__s8",
  i16: "__s16",
  i32: "__s32",
  i64: "__s64",
  bool: "__u8",
};

function cType(type: BpfType): string {
  if (type.kind === "scalar") return scalarCType[type.name];
  if (type.kind === "named") return `struct ${type.name}`;
  throw new BpfTsCompileError("byte arrays require a declarator context");
}

function fieldDeclaration(name: string, type: BpfType) {
  if (type.kind === "bytes") return `unsigned char ${name}[${type.length}];`;
  return `${cType(type)} ${name};`;
}

function mapDefinition(map: MapIR) {
  const lines = ["struct {"];
  const bpfType =
    map.kind === "ringbuf"
      ? "BPF_MAP_TYPE_RINGBUF"
      : map.kind === "hash"
        ? "BPF_MAP_TYPE_HASH"
        : "BPF_MAP_TYPE_ARRAY";
  lines.push(`  __uint(type, ${bpfType});`);
  lines.push(`  __uint(max_entries, ${map.maxEntries});`);
  if (map.kind !== "ringbuf") {
    if (!map.keyType) throw new BpfTsCompileError(`map ${map.name} is missing a key type`);
    lines.push(`  __type(key, ${cType(map.keyType)});`);
    lines.push(`  __type(value, ${cType(map.valueType)});`);
  }
  lines.push(`} ${map.name} SEC(".maps");`);
  return lines.join("\n");
}

function exprName(expr: ExprIR): string | null {
  if (expr.kind === "identifier") return expr.name;
  if (expr.kind === "property") {
    const parent = exprName(expr.object);
    return parent ? `${parent}.${expr.property}` : null;
  }
  return null;
}

function inferExprType(expr: ExprIR): BpfType {
  if (expr.kind === "boolean") return { kind: "scalar", name: "bool" };
  if (expr.kind === "call") {
    if (["bpf.pid", "bpf.tid", "bpf.uid", "bpf.gid"].includes(expr.callee)) {
      return { kind: "scalar", name: "u32" };
    }
    if (["bpf.ktimeNs", "bpf.arg"].includes(expr.callee)) {
      return { kind: "scalar", name: "u64" };
    }
  }
  return { kind: "scalar", name: "u64" };
}

function emitExpr(expr: ExprIR): string {
  switch (expr.kind) {
    case "number":
      return Number.isInteger(expr.value) ? String(expr.value) : `${expr.value}`;
    case "boolean":
      return expr.value ? "1" : "0";
    case "identifier":
      return expr.name;
    case "property":
      return `${emitExpr(expr.object)}.${expr.property}`;
    case "binary": {
      const op = expr.op === "===" ? "==" : expr.op === "!==" ? "!=" : expr.op;
      return `(${emitExpr(expr.left)} ${op} ${emitExpr(expr.right)})`;
    }
    case "unary":
      return `(${expr.op}${emitExpr(expr.value)})`;
    case "call": {
      switch (expr.callee) {
        case "bpf.pid":
          return "((__u32)(bpf_get_current_pid_tgid() >> 32))";
        case "bpf.tid":
          return "((__u32)bpf_get_current_pid_tgid())";
        case "bpf.uid":
          return "((__u32)bpf_get_current_uid_gid())";
        case "bpf.gid":
          return "((__u32)(bpf_get_current_uid_gid() >> 32))";
        case "bpf.ktimeNs":
          return "bpf_ktime_get_ns()";
        case "bpf.arg": {
          if (expr.args.length !== 2 || expr.args[1].kind !== "number") {
            throw new BpfTsCompileError("bpf.arg(ctx, N) requires a numeric argument index");
          }
          const index = expr.args[1].value;
          if (!Number.isInteger(index) || index < 1 || index > 5) {
            throw new BpfTsCompileError("bpf.arg supports argument indexes 1 through 5 in the initial backend");
          }
          return `PT_REGS_PARM${index}(${emitExpr(expr.args[0])})`;
        }
        default:
          throw new BpfTsCompileError(`call '${expr.callee}' is not valid in an expression`);
      }
    }
    case "object":
      throw new BpfTsCompileError("object literals are only valid as ringbuf.emit() payloads");
  }
}

class CEmitter {
  private counter = 0;
  private readonly maps = new Map<string, MapIR>();

  constructor(private readonly program: ProgramIR) {
    for (const map of program.maps) this.maps.set(map.name, map);
  }

  private temp(prefix: string) {
    this.counter += 1;
    return `__bpf_ts_${prefix}_${this.counter}`;
  }

  private emitRingbuf(map: MapIR, payload: ExprIR, indent: string): string[] {
    if (map.valueType.kind !== "named") {
      throw new BpfTsCompileError(`ringbuf ${map.name} must use a named struct value type`);
    }
    if (payload.kind !== "object") throw new BpfTsCompileError(`${map.name}.emit() requires an object literal`);
    const struct = this.program.structs.find((candidate) => candidate.name === map.valueType.name);
    if (!struct) throw new BpfTsCompileError(`unknown ringbuf value struct '${map.valueType.name}'`);
    const allowed = new Map(struct.fields.map((field) => [field.name, field]));
    const event = this.temp("event");
    const lines = [
      `${indent}struct ${map.valueType.name} *${event} = bpf_ringbuf_reserve(&${map.name}, sizeof(*${event}), 0);`,
      `${indent}if (${event}) {`,
      `${indent}  __builtin_memset(${event}, 0, sizeof(*${event}));`,
    ];
    for (const field of payload.fields) {
      const definition = allowed.get(field.name);
      if (!definition) throw new BpfTsCompileError(`struct ${struct.name} has no field '${field.name}'`);
      if (definition.type.kind === "bytes") {
        if (field.value.kind === "call" && field.value.callee === "bpf.comm" && field.value.args.length === 0) {
          lines.push(`${indent}  bpf_get_current_comm(${event}->${field.name}, sizeof(${event}->${field.name}));`);
          continue;
        }
        if (field.value.kind === "call" && field.value.callee === "bpf.userString" && field.value.args.length === 1) {
          lines.push(`${indent}  bpf_probe_read_user_str(${event}->${field.name}, sizeof(${event}->${field.name}), (const void *)(${emitExpr(field.value.args[0])}));`);
          continue;
        }
        throw new BpfTsCompileError(`byte field '${field.name}' requires bpf.comm() or bpf.userString(ptr)`);
      }
      lines.push(`${indent}  ${event}->${field.name} = ${emitExpr(field.value)};`);
    }
    lines.push(`${indent}  bpf_ringbuf_submit(${event}, 0);`, `${indent}}`);
    return lines;
  }

  private emitMapCall(call: Extract<ExprIR, { kind: "call" }>, indent: string): string[] | null {
    const dot = call.callee.lastIndexOf(".");
    if (dot <= 0) return null;
    const mapName = call.callee.slice(0, dot);
    const method = call.callee.slice(dot + 1);
    const map = this.maps.get(mapName);
    if (!map) return null;
    if (method === "emit") {
      if (map.kind !== "ringbuf" || call.args.length !== 1) throw new BpfTsCompileError(`${mapName}.emit(payload) is only valid for ringbuf maps`);
      return this.emitRingbuf(map, call.args[0], indent);
    }
    if (method === "set") {
      if (map.kind === "ringbuf" || call.args.length !== 2 || !map.keyType) {
        throw new BpfTsCompileError(`${mapName}.set(key, value) requires a hash/array map and two arguments`);
      }
      const key = this.temp("key");
      const value = this.temp("value");
      return [
        `${indent}${cType(map.keyType)} ${key} = ${emitExpr(call.args[0])};`,
        `${indent}${cType(map.valueType)} ${value} = ${emitExpr(call.args[1])};`,
        `${indent}bpf_map_update_elem(&${mapName}, &${key}, &${value}, BPF_ANY);`,
      ];
    }
    if (method === "increment") {
      if (map.kind === "ringbuf" || call.args.length !== 1 || !map.keyType || map.valueType.kind !== "scalar" || map.valueType.name !== "u64") {
        throw new BpfTsCompileError(`${mapName}.increment(key) requires a hash/array map with u64 values`);
      }
      const key = this.temp("key");
      const value = this.temp("value");
      const one = this.temp("one");
      return [
        `${indent}${cType(map.keyType)} ${key} = ${emitExpr(call.args[0])};`,
        `${indent}__u64 *${value} = bpf_map_lookup_elem(&${mapName}, &${key});`,
        `${indent}if (${value}) {`,
        `${indent}  __sync_fetch_and_add(${value}, 1);`,
        `${indent}} else {`,
        `${indent}  __u64 ${one} = 1;`,
        `${indent}  bpf_map_update_elem(&${mapName}, &${key}, &${one}, BPF_NOEXIST);`,
        `${indent}}`,
      ];
    }
    throw new BpfTsCompileError(`unsupported map operation '${call.callee}'`);
  }

  private emitHelperStatement(call: Extract<ExprIR, { kind: "call" }>, indent: string): string[] | null {
    if (call.callee === "bpf.readUser" && call.args.length === 2) {
      const target = exprName(call.args[0]);
      if (!target) throw new BpfTsCompileError("bpf.readUser(target, ptr) requires an addressable target");
      return [`${indent}bpf_probe_read_user(&${target}, sizeof(${target}), (const void *)(${emitExpr(call.args[1])}));`];
    }
    return null;
  }

  private emitStmt(stmt: StmtIR, depth: number): string[] {
    const indent = "  ".repeat(depth);
    switch (stmt.kind) {
      case "let": {
        if (stmt.value.kind === "object") throw new BpfTsCompileError("local object literals are not supported");
        const type = stmt.type ?? inferExprType(stmt.value);
        if (type.kind === "bytes") return [`${indent}unsigned char ${stmt.name}[${type.length}] = {};`];
        return [`${indent}${cType(type)} ${stmt.name} = ${emitExpr(stmt.value)};`];
      }
      case "assign":
        return [`${indent}${emitExpr(stmt.target)} ${stmt.op ?? "="} ${emitExpr(stmt.value)};`];
      case "return":
        return [`${indent}return ${emitExpr(stmt.value)};`];
      case "expr": {
        if (stmt.value.kind !== "call") return [`${indent}(void)(${emitExpr(stmt.value)});`];
        const mapLines = this.emitMapCall(stmt.value, indent);
        if (mapLines) return mapLines;
        const helperLines = this.emitHelperStatement(stmt.value, indent);
        if (helperLines) return helperLines;
        return [`${indent}(void)(${emitExpr(stmt.value)});`];
      }
      case "if": {
        const lines = [`${indent}if (${emitExpr(stmt.test)}) {`];
        for (const child of stmt.then) lines.push(...this.emitStmt(child, depth + 1));
        if (stmt.otherwise.length > 0) {
          lines.push(`${indent}} else {`);
          for (const child of stmt.otherwise) lines.push(...this.emitStmt(child, depth + 1));
        }
        lines.push(`${indent}}`);
        return lines;
      }
      case "for": {
        const lines = [
          `${indent}#pragma unroll`,
          `${indent}for (int ${stmt.variable} = ${stmt.start}; ${stmt.variable} < ${stmt.endExclusive}; ${stmt.variable} += ${stmt.step}) {`,
        ];
        for (const child of stmt.body) lines.push(...this.emitStmt(child, depth + 1));
        lines.push(`${indent}}`);
        return lines;
      }
    }
  }

  private probeSection(attach: ProgramIR["probes"][number]["attach"]) {
    if (attach.kind === "kprobe") return `kprobe/${attach.target}`;
    if (attach.kind === "tracepoint") return `tracepoint/${attach.category}/${attach.event}`;
    return "uprobe";
  }

  private probeComment(attach: ProgramIR["probes"][number]["attach"]) {
    if (attach.kind === "uprobe") return `// bpf-ts attach: uprobe symbol=${attach.target}`;
    return `// bpf-ts attach: ${this.probeSection(attach)}`;
  }

  generate() {
    const out: string[] = [
      "// Code generated by bpf-ts. DO NOT EDIT.",
      "#include <linux/bpf.h>",
      "#include <bpf/bpf_helpers.h>",
      "#include <bpf/bpf_tracing.h>",
      "",
    ];
    for (const struct of this.program.structs) {
      out.push(`struct ${struct.name} {`);
      for (const field of struct.fields) out.push(`  ${fieldDeclaration(field.name, field.type)}`);
      out.push("};", "");
    }
    for (const map of this.program.maps) out.push(mapDefinition(map), "");
    for (const probe of this.program.probes) {
      const context = probe.attach.kind === "tracepoint" ? "void *ctx" : "struct pt_regs *ctx";
      out.push(this.probeComment(probe.attach));
      out.push(`SEC("${this.probeSection(probe.attach)}")`);
      out.push(`int ${probe.name}(${context}) {`);
      for (const stmt of probe.body) out.push(...this.emitStmt(stmt, 1));
      out.push("}", "");
    }
    out.push('char LICENSE[] SEC("license") = "Dual BSD/GPL";', "");
    return out.join("\n");
  }
}

export function generateBpfC(program: ProgramIR) {
  return new CEmitter(program).generate();
}
