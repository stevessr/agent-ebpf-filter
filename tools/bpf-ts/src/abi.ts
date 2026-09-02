import { BpfTsCompileError } from "./diagnostics";
import type { BpfType, ProgramIR } from "./ir";

type Layout = {
  size: number;
  align: number;
};

const scalarLayout: Record<string, Layout> = {
  u8: { size: 1, align: 1 },
  i8: { size: 1, align: 1 },
  bool: { size: 1, align: 1 },
  u16: { size: 2, align: 2 },
  i16: { size: 2, align: 2 },
  u32: { size: 4, align: 4 },
  i32: { size: 4, align: 4 },
  u64: { size: 8, align: 8 },
  i64: { size: 8, align: 8 },
};

function alignUp(value: number, alignment: number): number {
  return Math.ceil(value / alignment) * alignment;
}

function layoutOfType(
  program: ProgramIR,
  type: BpfType,
  stack: Set<string>,
): Layout {
  if (type.kind === "scalar") return scalarLayout[type.name];
  if (type.kind === "bytes") return { size: type.length, align: 1 };

  const struct = program.structs.find((candidate) => candidate.name === type.name);
  if (!struct) {
    throw new BpfTsCompileError(`cannot compute ABI layout for unknown struct '${type.name}'`);
  }
  if (struct.core) {
    throw new BpfTsCompileError(
      `CO-RE projection '${type.name}' cannot be used as a concrete map ABI value`,
    );
  }
  if (stack.has(struct.name)) {
    throw new BpfTsCompileError(`recursive struct '${struct.name}' has no finite ABI size`);
  }

  stack.add(struct.name);
  let offset = 0;
  let structAlign = 1;
  for (const field of struct.fields) {
    const fieldLayout = layoutOfType(program, field.type, stack);
    offset = alignUp(offset, fieldLayout.align);
    offset += fieldLayout.size;
    structAlign = Math.max(structAlign, fieldLayout.align);
  }
  stack.delete(struct.name);

  return {
    size: alignUp(offset, structAlign),
    align: structAlign,
  };
}

export function bpfTypeSize(program: ProgramIR, type: BpfType): number {
  const { size } = layoutOfType(program, type, new Set());
  if (!Number.isSafeInteger(size) || size <= 0 || size > 0xffffffff) {
    throw new BpfTsCompileError(`BPF ABI type size ${size} is outside the u32 map ABI range`);
  }
  return size;
}
