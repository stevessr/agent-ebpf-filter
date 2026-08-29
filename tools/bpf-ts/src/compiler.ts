import { generateBpfC } from "./codegen";
import { parseBpfTs } from "./parser";
import { validateBpfProgram } from "./validate";
import type { ProbeAttachIR, ProgramIR } from "./ir";

function probeSection(attach: ProbeAttachIR): string {
  if (attach.kind === "kprobe") return `kprobe/${attach.target}`;
  if (attach.kind === "tracepoint") return `tracepoint/${attach.category}/${attach.event}`;
  return "uprobe";
}

export interface BpfTsManifest {
  version: 1;
  source: string;
  probes: Array<{
    name: string;
    section: string;
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
      probes: ir.probes.map((probe) => ({
        name: probe.name,
        section: probeSection(probe.attach),
        ...probe.attach,
      })),
      maps: ir.maps.map((map) => ({
        name: map.name,
        kind: map.kind,
        maxEntries: map.maxEntries,
      })),
    },
  };
}
