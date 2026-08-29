import { BpfTsCompileError } from "./diagnostics";
import { ringbufScratchMapName } from "./ringbufscratch";
import type { BpfType, ExprIR, MapIR, ProgramIR, StmtIR, StructIR } from "./ir";

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

type CompactRingbufTail = {
  struct: StructIR;
  tail: StructIR["fields"][number];
  tailCall: Extract<ExprIR, { kind: "call" }>;
  scratch: MapIR;
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
        : map.kind === "percpu_array"
          ? "BPF_MAP_TYPE_PERCPU_ARRAY"
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
    if (expr.callee === "bpf.retI32") return { kind: "scalar", name: "i32" };
    if (expr.callee === "bpf.ret") return { kind: "scalar", name: "i64" };
    if (
      ["bpf.ktimeNs", "bpf.arg", "bpf.currentTask"].includes(expr.callee) ||
      expr.callee.startsWith("bpf.coreRead.")
    ) {
      return { kind: "scalar", name: "u64" };
    }
  }
  return { kind: "scalar", name: "u64" };
}

function emitCoreRead(expr: Extract<ExprIR, { kind: "call" }>): string {
  const parts = expr.callee.split(".");
  if (parts.length !== 4 || parts[0] !== "bpf" || parts[1] !== "coreRead" || expr.args.length !== 1) {
    throw new BpfTsCompileError(
      `invalid CO-RE helper '${expr.callee}'; use bpf.coreRead.<Struct>.<field>(pointer)`,
    );
  }
  const structName = parts[2];
  const fieldName = parts[3];
  return `BPF_CORE_READ((struct ${structName} *)(unsigned long)(${emitExpr(expr.args[0])}), ${fieldName})`;
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
      if (expr.callee.startsWith("bpf.coreRead.")) return emitCoreRead(expr);
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
        case "bpf.currentTask":
          return "((__u64)(unsigned long)bpf_get_current_task())";
        case "bpf.ret":
          if (expr.args.length !== 1) throw new BpfTsCompileError("bpf.ret(ctx) requires one context argument");
          return `((__s64)PT_REGS_RC(${emitExpr(expr.args[0])}))`;
        case "bpf.retI32":
          if (expr.args.length !== 1) throw new BpfTsCompileError("bpf.retI32(ctx) requires one context argument");
          return `((__s32)PT_REGS_RC(${emitExpr(expr.args[0])}))`;
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
          throw new BpfTsCompileError(`call '${expr.callee}' is not valid in an inline expression`);
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

  private structForRingbuf(map: MapIR): StructIR {
    const valueType = map.valueType;
    if (valueType.kind !== "named") {
      throw new BpfTsCompileError(`ringbuf ${map.name} must use a named struct value type`);
    }
    const struct = this.program.structs.find((candidate) => candidate.name === valueType.name);
    if (!struct) throw new BpfTsCompileError(`unknown ringbuf value struct '${valueType.name}'`);
    return struct;
  }

  private emitByteField(
    event: string,
    fieldName: string,
    value: ExprIR,
    indent: string,
  ): string[] {
    if (value.kind === "call" && value.callee === "bpf.comm" && value.args.length === 0) {
      return [
        `${indent}bpf_get_current_comm(${event}->${fieldName}, sizeof(${event}->${fieldName}));`,
      ];
    }
    if (value.kind === "call" && value.callee === "bpf.userString" && value.args.length === 1) {
      return [
        `${indent}bpf_probe_read_user_str(${event}->${fieldName}, sizeof(${event}->${fieldName}), (const void *)(${emitExpr(value.args[0])}));`,
      ];
    }
    if (value.kind === "call" && value.callee === "bpf.userBytes" && value.args.length === 2) {
      const readLen = this.temp("read_len");
      return [
        `${indent}__u64 ${readLen} = ${emitExpr(value.args[1])};`,
        `${indent}if (${readLen} > sizeof(${event}->${fieldName})) ${readLen} = sizeof(${event}->${fieldName});`,
        `${indent}if (${readLen} > 0) {`,
        `${indent}  bpf_probe_read_user(${event}->${fieldName}, (__u32)${readLen}, (const void *)(${emitExpr(value.args[0])}));`,
        `${indent}}`,
      ];
    }
    throw new BpfTsCompileError(
      `byte field '${fieldName}' requires bpf.comm(), bpf.userString(ptr), or bpf.userBytes(ptr, len)`,
    );
  }

  private compactRingbufTail(
    map: MapIR,
    payload: Extract<ExprIR, { kind: "object" }>,
  ): CompactRingbufTail | null {
    const scratch = this.maps.get(ringbufScratchMapName(map.name));
    if (!scratch || scratch.kind !== "percpu_array") return null;
    const struct = this.structForRingbuf(map);
    const tail = struct.fields.at(-1);
    if (!tail || tail.type.kind !== "bytes") return null;
    const payloadField = payload.fields.find((field) => field.name === tail.name);
    if (!payloadField) return null;
    const tailCall = payloadField.value;
    if (
      tailCall.kind !== "call" ||
      tailCall.callee !== "bpf.userBytes" ||
      tailCall.args.length !== 2
    ) {
      return null;
    }
    return { struct, tail, tailCall, scratch };
  }

  private emitCompactRingbuf(
    map: MapIR,
    payload: Extract<ExprIR, { kind: "object" }>,
    indent: string,
  ): string[] | null {
    const compact = this.compactRingbufTail(map, payload);
    if (!compact) return null;

    const { struct, tail, tailCall, scratch } = compact;
    const allowed = new Map(struct.fields.map((field) => [field.name, field]));
    const key = this.temp("scratch_key");
    const event = this.temp("event");
    const readLen = this.temp("read_len");
    const readRC = this.temp("read_rc");
    const recordLen = this.temp("record_len");
    const headerSize = `__builtin_offsetof(struct ${struct.name}, ${tail.name})`;
    const lines = [
      `${indent}__u32 ${key} = 0;`,
      `${indent}struct ${struct.name} *${event} = bpf_map_lookup_elem(&${scratch.name}, &${key});`,
      `${indent}if (${event}) {`,
      // Zero the complete emitted prefix, including C padding. The large tail is
      // never emitted beyond readLen and therefore does not need a 4 KiB memset.
      `${indent}  __builtin_memset(${event}, 0, ${headerSize});`,
    ];

    for (const field of payload.fields) {
      const definition = allowed.get(field.name);
      if (!definition) throw new BpfTsCompileError(`struct ${struct.name} has no field '${field.name}'`);
      if (field.name === tail.name) continue;
      if (definition.type.kind === "bytes") {
        lines.push(...this.emitByteField(event, field.name, field.value, `${indent}  `));
      } else {
        lines.push(`${indent}  ${event}->${field.name} = ${emitExpr(field.value)};`);
      }
    }

    lines.push(
      `${indent}  __u64 ${readLen} = ${emitExpr(tailCall.args[1])};`,
      `${indent}  if (${readLen} > sizeof(${event}->${tail.name})) ${readLen} = sizeof(${event}->${tail.name});`,
      `${indent}  if (${readLen} > 0) {`,
      `${indent}    long ${readRC} = bpf_probe_read_user(${event}->${tail.name}, (__u32)${readLen}, (const void *)(${emitExpr(tailCall.args[0])}));`,
      `${indent}    if (${readRC} == 0) {`,
      `${indent}      __u64 ${recordLen} = ${headerSize} + ${readLen};`,
      `${indent}      bpf_ringbuf_output(&${map.name}, ${event}, ${recordLen}, 0);`,
      `${indent}    }`,
      `${indent}  } else {`,
      `${indent}    bpf_ringbuf_output(&${map.name}, ${event}, ${headerSize}, 0);`,
      `${indent}  }`,
      `${indent}}`,
    );
    return lines;
  }

  private emitRingbuf(map: MapIR, payload: ExprIR, indent: string): string[] {
    if (payload.kind !== "object") throw new BpfTsCompileError(`${map.name}.emit() requires an object literal`);
    const compact = this.emitCompactRingbuf(map, payload, indent);
    if (compact) return compact;

    const struct = this.structForRingbuf(map);
    const allowed = new Map(struct.fields.map((field) => [field.name, field]));
    const event = this.temp("event");
    const lines = [
      `${indent}struct ${struct.name} *${event} = bpf_ringbuf_reserve(&${map.name}, sizeof(*${event}), 0);`,
      `${indent}if (${event}) {`,
      `${indent}  __builtin_memset(${event}, 0, sizeof(*${event}));`,
    ];
    for (const field of payload.fields) {
      const definition = allowed.get(field.name);
      if (!definition) throw new BpfTsCompileError(`struct ${struct.name} has no field '${field.name}'`);
      if (definition.type.kind === "bytes") {
        lines.push(...this.emitByteField(event, field.name, field.value, `${indent}  `));
      } else {
        lines.push(`${indent}  ${event}->${field.name} = ${emitExpr(field.value)};`);
      }
    }
    lines.push(`${indent}  bpf_ringbuf_submit(${event}, 0);`, `${indent}}`);
    return lines;
  }

  private mapCall(call: Extract<ExprIR, { kind: "call" }>) {
    const dot = call.callee.lastIndexOf(".");
    if (dot <= 0) return null;
    const mapName = call.callee.slice(0, dot);
    const map = this.maps.get(mapName);
    if (!map) return null;
    return { mapName, map, method: call.callee.slice(dot + 1) };
  }

  private emitMapLookupLet(stmt: Extract<StmtIR, { kind: "let" }>, indent: string): string[] | null {
    if (stmt.value.kind !== "call") return null;
    const resolved = this.mapCall(stmt.value);
    if (!resolved || resolved.method !== "getOr") return null;
    const { mapName, map } = resolved;
    if (map.kind === "ringbuf" || map.kind === "percpu_array" || !map.keyType || stmt.value.args.length !== 2) {
      throw new BpfTsCompileError(`${mapName}.getOr(key, fallback) requires a hash/array map`);
    }
    const key = this.temp("key");
    const found = this.temp("found");
    const type = stmt.type ?? map.valueType;
    if (type.kind !== "scalar" || map.valueType.kind !== "scalar") {
      throw new BpfTsCompileError(`${mapName}.getOr() currently supports scalar map values only`);
    }
    return [
      `${indent}${cType(map.keyType)} ${key} = ${emitExpr(stmt.value.args[0])};`,
      `${indent}${cType(map.valueType)} *${found} = bpf_map_lookup_elem(&${mapName}, &${key});`,
      `${indent}${cType(type)} ${stmt.name} = ${found} ? *${found} : (${cType(type)})(${emitExpr(stmt.value.args[1])});`,
    ];
  }

  private emitMapCall(call: Extract<ExprIR, { kind: "call" }>, indent: string): string[] | null {
    const resolved = this.mapCall(call);
    if (!resolved) return null;
    const { mapName, map, method } = resolved;
    if (method === "emit") {
      if (map.kind !== "ringbuf" || call.args.length !== 1) throw new BpfTsCompileError(`${mapName}.emit(payload) is only valid for ringbuf maps`);
      return this.emitRingbuf(map, call.args[0], indent);
    }
    if (method === "set") {
      if (map.kind === "ringbuf" || map.kind === "percpu_array" || call.args.length !== 2 || !map.keyType) {
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
    if (method === "delete") {
      if (map.kind !== "hash" || call.args.length !== 1 || !map.keyType) {
        throw new BpfTsCompileError(`${mapName}.delete(key) requires a hash map`);
      }
      const key = this.temp("key");
      return [
        `${indent}${cType(map.keyType)} ${key} = ${emitExpr(call.args[0])};`,
        `${indent}bpf_map_delete_elem(&${mapName}, &${key});`,
      ];
    }
    if (method === "getOr") {
      throw new BpfTsCompileError(`${mapName}.getOr() must be used as a direct local initializer`);
    }
    if (method === "increment") {
      if (
        map.kind === "ringbuf" ||
        map.kind === "percpu_array" ||
        call.args.length !== 1 ||
        !map.keyType ||
        map.valueType.kind !== "scalar" ||
        map.valueType.name !== "u64"
      ) {
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
        const lookup = this.emitMapLookupLet(stmt, indent);
        if (lookup) return lookup;
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
    switch (attach.kind) {
      case "kprobe":
        return `kprobe/${attach.target}`;
      case "kretprobe":
        return `kretprobe/${attach.target}`;
      case "uprobe":
        return "uprobe";
      case "uretprobe":
        return "uretprobe";
      case "tracepoint":
        return `tracepoint/${attach.category}/${attach.event}`;
    }
  }

  private probeComment(attach: ProgramIR["probes"][number]["attach"]) {
    if (attach.kind === "uprobe" || attach.kind === "uretprobe") {
      return `// bpf-ts attach: ${attach.kind} symbol=${attach.target}`;
    }
    return `// bpf-ts attach: ${this.probeSection(attach)}`;
  }

  generate() {
    const out: string[] = [
      "// Code generated by bpf-ts. DO NOT EDIT.",
      "#include <linux/bpf.h>",
      "#include <linux/ptrace.h>",
      "#include <bpf/bpf_helpers.h>",
      "#include <bpf/bpf_tracing.h>",
      "#include <bpf/bpf_core_read.h>",
      "",
    ];
    for (const struct of this.program.structs) {
      out.push(`struct ${struct.name} {`);
      for (const field of struct.fields) out.push(`  ${fieldDeclaration(field.name, field.type)}`);
      out.push("};", "");
    }
    for (const map of this.program.maps) out.push(mapDefinition(map), "");
    for (const probe of this.program.probes) {
      const context = probe.attach.kind === "tracepoint"
        ? `void *${probe.contextName}`
        : `struct pt_regs *${probe.contextName}`;
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
