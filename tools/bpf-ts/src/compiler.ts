import { generateBpfC } from "./codegen";
import { parseBpfTs } from "./parser";
import { validateBpfProgram } from "./validate";
import type { ProgramIR } from "./ir";

export interface BpfTsManifest {
  version: 1;
  source: string;
  probes: Array<{
    name: string;
    kind: "kprobe" | "uprobe" | "tracepoint";
    target?: string;
    category?: string;
    event?: string;
  }>;
  maps: Array<{
    name: string;
    kind: "ringbuf" | "hash" | "array";
    maxEntries: number;
  }>;
}

export interface BpfTsCompilation {
  ir: ProgramIR;
  cSource: string;
  manifest: BpfTsManifest;
}

export function compileBpfTs(sourceText: string, fileName = "program.ts"): BpfTsCompilation {
  const ir = parseBpfTs(sourceText, fileName);
  validateBpfProgram(ir);
  return {
    ir,
    cSource: generateBpfC(ir),
    manifest: {
      version: 1,
      source: fileName,
      probes: ir.probes.map((probe) => ({ name: probe.name, ...probe.attach })),
      maps: ir.maps.map((map) => ({
        name: map.name,
        kind: map.kind,
        maxEntries: map.maxEntries,
      })),
    },
  };
}
