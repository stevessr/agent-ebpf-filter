import { generateBpfC } from "./codegen";
import { validateCoreAccesses } from "./corevalidate";
import { markCoreTypeProjections } from "./coretypes";
import { validateMapExpressions } from "./mapexprvalidate";
import { parseBpfTs } from "./parser";
import { validatePayloadShapes } from "./payloadvalidate";
import { applyReturnProbeKinds, normalizeReturnProbeDecorators } from "./probedecorators";
import { validateVerifierResources } from "./resourcevalidate";
import { validateProbeSignatures } from "./signature";
import { inferLocalTypes } from "./typeinfer";
import { validateBpfProgram } from "./validate";
import type { ProbeAttachIR, ProbeKind, ProgramIR } from "./ir";

function probeSection(attach: ProbeAttachIR): string {
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

export interface BpfTsManifest {
  version: 1;
  source: string;
  probes: Array<{
    name: string;
    section: string;
    kind: ProbeKind;
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
  // TypeScript currently treats method decorators uniformly, but the existing
  // parser deliberately understands only entry probes. Normalize return-probe
  // decorators through the TypeScript AST, parse the normalized source, then
  // restore the stronger attach kind in IR before semantic validation/codegen.
  const normalized = normalizeReturnProbeDecorators(sourceText, fileName);
  validateProbeSignatures(normalized.sourceText, fileName);
  const ir = parseBpfTs(normalized.sourceText, fileName);
  applyReturnProbeKinds(ir, normalized.returnKinds);

  // CO-RE projections are source roles (declare interface), so retain the
  // author's original source rather than the pretty-printed normalized form.
  markCoreTypeProjections(sourceText, fileName, ir);
  validateBpfProgram(ir);
  validateCoreAccesses(ir);
  validateMapExpressions(ir);
  inferLocalTypes(ir);
  validatePayloadShapes(ir);
  validateVerifierResources(ir);
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
