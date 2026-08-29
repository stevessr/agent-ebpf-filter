import { lowerArgI32 } from "./argi32";
import { generateBpfC } from "./codegen";
import { validateCoreAccesses } from "./corevalidate";
import { markCoreTypeProjections } from "./coretypes";
import { validateMapExpressions } from "./mapexprvalidate";
import { parseBpfTs } from "./parser";
import { validatePayloadShapes } from "./payloadvalidate";
import { applyReturnProbeKinds, normalizeReturnProbeDecorators } from "./probedecorators";
import { validateVerifierResources } from "./resourcevalidate";
import { addRingbufDropMaps } from "./ringbufdrops";
import { addCompactRingbufScratchMaps } from "./ringbufscratch";
import { lowerLargeRingbufZeroing } from "./ringbufzero";
import { validateProbeSignatures } from "./signature";
import { lowerTakeOr } from "./takeor";
import { inferLocalTypes } from "./typeinfer";
import { validateBpfProgram } from "./validate";
import type { MapKind, ProbeAttachIR, ProbeKind, ProgramIR } from "./ir";

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
    kind: MapKind;
    maxEntries: number;
  }>;
}

export interface BpfTsCompilation {
  ir: ProgramIR;
  cSource: string;
  manifest: BpfTsManifest;
}

export function compileBpfTs(sourceText: string, fileName = "program.ts"): BpfTsCompilation {
  const normalized = normalizeReturnProbeDecorators(sourceText, fileName);
  validateProbeSignatures(normalized.sourceText, fileName);
  const ir = parseBpfTs(normalized.sourceText, fileName);
  applyReturnProbeKinds(ir, normalized.returnKinds);

  // CO-RE projections are source roles (declare interface), so retain the
  // author's original source rather than the pretty-printed normalized form.
  markCoreTypeProjections(sourceText, fileName, ir);

  // takeOr is a verifier-friendly semantic primitive. Lower it before ordinary
  // map validation so the C backend only needs the already-tested getOr/delete
  // operations and cannot accidentally retain a map-value pointer after delete.
  lowerTakeOr(ir);
  validateBpfProgram(ir);
  validateCoreAccesses(ir);
  validateMapExpressions(ir);
  inferLocalTypes(ir);

  // Signed ABI arguments intentionally reuse the existing PT_REGS_PARMn C
  // lowering. Infer their locals as i32 first, then lower argI32 to arg so the
  // generated __s32 assignment performs the required truncation/sign handling.
  lowerArgI32(ir);

  validatePayloadShapes(ir);
  validateVerifierResources(ir);

  // Compiler-owned maps are appended only after validating the author's public
  // DSL. This keeps structured/internal map kinds out of user map semantics.
  addCompactRingbufScratchMaps(ir);
  addRingbufDropMaps(ir);

  // Keep the older large-record zeroing rewrite as a fail-safe for large
  // ringbuf records that cannot use the compact trailing-bytes path.
  const cSource = lowerLargeRingbufZeroing(ir, generateBpfC(ir));
  return {
    ir,
    cSource,
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
