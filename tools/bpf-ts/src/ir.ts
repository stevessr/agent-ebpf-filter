export type ScalarType =
  | "u8"
  | "u16"
  | "u32"
  | "u64"
  | "i8"
  | "i16"
  | "i32"
  | "i64"
  | "bool";

export type BpfType =
  | { kind: "scalar"; name: ScalarType }
  | { kind: "bytes"; length: number }
  | { kind: "named"; name: string };

export interface StructField {
  name: string;
  type: BpfType;
}

export interface StructIR {
  name: string;
  fields: StructField[];
  core?: boolean;
}

export type MapKind = "ringbuf" | "hash" | "array";

export interface MapIR {
  name: string;
  kind: MapKind;
  keyType?: BpfType;
  valueType: BpfType;
  maxEntries: number;
}

export type ProbeKind = "kprobe" | "kretprobe" | "uprobe" | "uretprobe" | "tracepoint";

export interface ProbeAttachIR {
  kind: ProbeKind;
  target?: string;
  category?: string;
  event?: string;
}

export type ExprIR =
  | { kind: "number"; value: number }
  | { kind: "boolean"; value: boolean }
  | { kind: "identifier"; name: string }
  | { kind: "property"; object: ExprIR; property: string }
  | { kind: "binary"; op: string; left: ExprIR; right: ExprIR }
  | { kind: "unary"; op: string; value: ExprIR }
  | { kind: "call"; callee: string; args: ExprIR[] }
  | { kind: "object"; fields: Array<{ name: string; value: ExprIR }> };

export type StmtIR =
  | { kind: "let"; name: string; type?: BpfType; value: ExprIR }
  | { kind: "assign"; target: ExprIR; value: ExprIR; op?: string }
  | { kind: "expr"; value: ExprIR }
  | { kind: "return"; value: ExprIR }
  | { kind: "if"; test: ExprIR; then: StmtIR[]; otherwise: StmtIR[] }
  | {
      kind: "for";
      variable: string;
      start: number;
      endExclusive: number;
      step: number;
      body: StmtIR[];
    };

export interface ProbeIR {
  name: string;
  attach: ProbeAttachIR;
  contextName: string;
  contextType: string;
  body: StmtIR[];
}

export interface ProgramIR {
  structs: StructIR[];
  maps: MapIR[];
  probes: ProbeIR[];
}
