/**
 * Reproducible CPU / heap benchmark for the presentation-only Harness graph.
 * Usage: bun scripts/benchmark-harness-capture.ts
 *
 * This is measurement, not a machine-dependent CI latency threshold.
 * It uses only synthetic metadata and never accesses the real collector.
 */
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { performance } from "node:perf_hooks";

import { buildHarnessCaptureGraph } from "../src/components/agentsight/harnessCaptureGraph";
import { selectBoundedCaptureEvents } from "../src/components/agentsight/harnessCaptureSelection";
import type { AgentSightEvent } from "../src/utils/agentsight/types";

const total = 50_000;
const selectedLimit = 1_000;
const samples: AgentSightEvent[] = Array.from({ length: total }, (_, i) => {
  const isProtocol = i % 5 === 0;
  const isCritical = i % 1000 === 0;
  return {
    id: "synthetic-" + i,
    timestamp: 1_800_000_000_000 + i,
    source: isProtocol ? "stdio" : isCritical ? "policy" : "system",
    rawSource: isProtocol ? "stdio" : isCritical ? "policy" : "system",
    pid: 100 + (i % 8),
    ppid: 1,
    comm: "synthetic-agent",
    eventType: isProtocol ? "STDIO" : isCritical ? "ALERT" : "system_metrics",
    traceId: "synthetic-trace",
    spanId: "",
    redactionState: "sanitized",
    title: "synthetic fixture",
    data: isProtocol ? {
      fd: 0, direction: "write",
      data: JSON.stringify({ jsonrpc: "2.0", id: i,
        method: "tools/call", params: { name: "synthetic_tool" } }) + "\n",
    } : { alert: isCritical, type: "system_metrics" },
    raw: {},
  };
});

const priority = (event: AgentSightEvent) =>
  event.source === "policy" ? 0 : event.source === "stdio" ? 1 : 3;
const identity = (event: AgentSightEvent) => event.id;
const compare = (a: AgentSightEvent, b: AgentSightEvent) =>
  priority(a) - priority(b) || b.timestamp - a.timestamp ||
  identity(a).localeCompare(identity(b));
const chrono = (a: AgentSightEvent, b: AgentSightEvent) =>
  a.timestamp - b.timestamp || identity(a).localeCompare(identity(b));
const median = (data: number[]) =>
  data.slice().sort((a, b) => a - b)[Math.floor(data.length / 2)];
const measure = (fn: () => unknown, iterations = 4) => {
  const elapsed: number[] = [];
  for (let i = 0; i < iterations; i++) {
    const before = performance.now();
    fn();
    elapsed.push(performance.now() - before);
  }
  return {
    medianMs: Number(median(elapsed).toFixed(2)),
    samplesMs: elapsed.map((n) => Number(n.toFixed(2))),
  };
};

const heapSelected = selectBoundedCaptureEvents(samples, selectedLimit, priority, identity);
const fullSelected = samples.slice().sort(compare).slice(0, selectedLimit).sort(chrono);
assert.deepEqual(heapSelected.map(identity), fullSelected.map(identity));
const graph = buildHarnessCaptureGraph(samples, selectedLimit);
assert.equal(graph.displayedEvents, selectedLimit);
assert.equal(graph.omittedEvents, total - selectedLimit);
assert(graph.nodes.length >= selectedLimit);
const report = {
  name: "Harness capture graph synthetic CPU benchmark",
  totalEvents: total,
  drawnEvents: selectedLimit,
  runtime: process.version,
  selection: {
    boundedHeap: measure(() =>
      selectBoundedCaptureEvents(samples, selectedLimit, priority, identity)),
    fullArraySort: measure(() =>
      samples.slice().sort(compare).slice(0, selectedLimit).sort(chrono)),
  },
  graphBuild: measure(() => buildHarnessCaptureGraph(samples, selectedLimit), 3),
  graphSizes: {
    nodes: graph.nodes.length,
    edges: graph.edges.length,
    omittedMessages: graph.omittedMessages,
  },
  heapUsedMiB: Number((process.memoryUsage().heapUsed / 1024 / 1024).toFixed(2)),
};
const outdir = path.resolve("artifacts/harness-benchmarks");
fs.mkdirSync(outdir, { recursive: true });
fs.writeFileSync(path.join(outdir, "harness-graph.json"),
  JSON.stringify(report, null, 2) + "\n");
console.log(JSON.stringify(report, null, 2));
