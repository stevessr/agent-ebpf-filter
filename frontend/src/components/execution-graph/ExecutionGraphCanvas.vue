<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import * as d3 from "d3";

import type {
  ExecutionGraphEdge,
  ExecutionGraphNode,
} from "../../types/executionGraph";
import {
  kindColor,
  nodeRadius,
  truncate,
  linkStrokeWidth,
  linkStrokeColor,
  linkDistance,
  linkStrength,
} from "./useExecutionGraphHelpers";
import {
  buildDisplayGraph,
  buildForceNodes,
  applyProcessTreeLayout,
  createTopologyKey,
  type ForceNode,
  type ForceLink,
} from "./useDisplayGraphBuilder";

const props = withDefaults(
  defineProps<{
    nodes: ExecutionGraphNode[];
    edges: ExecutionGraphEdge[];
    selectedNodeId?: string;
    height?: number;
    zoomStorageKey?: string;
  }>(),
  {
    selectedNodeId: "",
    height: 620,
    zoomStorageKey: "agent-ebpf.execution-graph.zoom",
  },
);

const emit = defineEmits<{
  (event: "select-node", nodeId: string): void;
}>();

const containerRef = ref<HTMLDivElement | null>(null);
const svgRef = ref<SVGSVGElement | null>(null);
let simulation: d3.Simulation<ForceNode, ForceLink> | null = null;
let rootGroup: d3.Selection<SVGGElement, unknown, null, undefined> | null =
  null;
let linkGroup: d3.Selection<SVGGElement, unknown, null, undefined> | null =
  null;
let nodeGroup: d3.Selection<SVGGElement, unknown, null, undefined> | null =
  null;
let emptyGroup: d3.Selection<SVGGElement, unknown, null, undefined> | null =
  null;
let zoomBehavior: d3.ZoomBehavior<SVGSVGElement, unknown> | null = null;
let resizeObserver: ResizeObserver | null = null;
let pendingRenderFrame = 0;
let initialZoomApplied = false;
let lastTopologyKey = "";
let currentDisplayGraph: {
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
} = { nodes: [], edges: [] };

type SavedNodePosition = { x: number; y: number };

const nodePositionStorageKey = () =>
  `${props.zoomStorageKey || "agent-ebpf.execution-graph.zoom"}.node-positions`;

const loadSavedNodePositions = () => {
  const positions = new Map<string, SavedNodePosition>();
  try {
    const raw = localStorage.getItem(nodePositionStorageKey());
    if (!raw) return positions;
    const parsed = JSON.parse(raw) as Record<
      string,
      Partial<SavedNodePosition>
    >;
    Object.entries(parsed).forEach(([id, value]) => {
      const x = Number(value.x);
      const y = Number(value.y);
      if (id && Number.isFinite(x) && Number.isFinite(y)) {
        positions.set(id, { x, y });
      }
    });
  } catch {
    // Ignore corrupted or unavailable storage; the graph can fall back to stable layout.
  }
  return positions;
};

const persistNodePosition = (node: ForceNode) => {
  const x = Number(node.x ?? node.fx);
  const y = Number(node.y ?? node.fy);
  if (!Number.isFinite(x) || !Number.isFinite(y)) return;
  try {
    const positions = Object.fromEntries(loadSavedNodePositions());
    positions[node.id] = { x, y };
    localStorage.setItem(nodePositionStorageKey(), JSON.stringify(positions));
  } catch {
    // Ignore storage quota / privacy mode failures.
  }
};

const loadPersistedZoom = (width: number, height: number) => {
  if (!props.zoomStorageKey) return d3.zoomIdentity;
  try {
    const raw = localStorage.getItem(props.zoomStorageKey);
    if (!raw) return d3.zoomIdentity;
    const parsed = JSON.parse(raw) as { x?: unknown; y?: unknown; k?: unknown };
    const x = Number(parsed.x);
    const y = Number(parsed.y);
    const k = Number(parsed.k);
    const minVisibleX = -width * 1.8;
    const maxVisibleX = width * 1.2;
    const minVisibleY = -height * 1.8;
    const maxVisibleY = height * 1.2;
    if (
      !Number.isFinite(x) ||
      !Number.isFinite(y) ||
      !Number.isFinite(k) ||
      k <= 0 ||
      x < minVisibleX ||
      x > maxVisibleX ||
      y < minVisibleY ||
      y > maxVisibleY
    ) {
      return d3.zoomIdentity;
    }
    return d3.zoomIdentity.translate(x, y).scale(k);
  } catch {
    return d3.zoomIdentity;
  }
};

const persistZoom = (transform: d3.ZoomTransform) => {
  if (!props.zoomStorageKey) return;
  try {
    localStorage.setItem(
      props.zoomStorageKey,
      JSON.stringify({ x: transform.x, y: transform.y, k: transform.k }),
    );
  } catch {
    // Ignore storage quota / privacy mode failures.
  }
};

const initializeCanvas = (
  svgElement: SVGSVGElement,
  width: number,
  height: number,
) => {
  const svg = d3.select(svgElement);
  svg.attr("viewBox", `0 0 ${width} ${height}`);

  if (!rootGroup || !linkGroup || !nodeGroup || !emptyGroup || !zoomBehavior) {
    rootGroup = svg.append("g");
    linkGroup = rootGroup
      .append("g")
      .attr("stroke", "#cbd5e1")
      .attr("stroke-opacity", 0.75);
    nodeGroup = rootGroup.append("g");
    emptyGroup = rootGroup.append("g");

    emptyGroup
      .append("text")
      .attr("text-anchor", "middle")
      .attr("fill", "#64748b")
      .attr("font-size", 14)
      .text("No graph data");

    zoomBehavior = d3
      .zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.35, 2.5])
      .on("zoom", (event) => {
        rootGroup?.attr("transform", event.transform.toString());
        persistZoom(event.transform);
      });
    svg.call(zoomBehavior);
  }

  if (!initialZoomApplied && zoomBehavior) {
    svg.call(zoomBehavior.transform, loadPersistedZoom(width, height));
    initialZoomApplied = true;
  }
};

const updateEmptyState = (width: number, height: number, hasNodes: boolean) => {
  if (!emptyGroup) return;
  emptyGroup.style("display", hasNodes ? "none" : "");
  emptyGroup
    .select("text")
    .attr("x", width / 2)
    .attr("y", height / 2);
};

const updateSimulation = (
  nodes: ForceNode[],
  links: ForceLink[],
  width: number,
  height: number,
  topologyChanged: boolean,
) => {
  const simulationLinks = d3
    .forceLink<ForceNode, ForceLink>(links)
    .id((node) => node.id)
    .distance((link) => linkDistance(link.kind))
    .strength((link) => linkStrength(link.kind));

  if (!simulation) {
    simulation = d3
      .forceSimulation<ForceNode>(nodes)
      .force("link", simulationLinks)
      .force("charge", d3.forceManyBody<ForceNode>().strength(-340))
      .force("center", d3.forceCenter(width / 2, height / 2))
      .force(
        "collision",
        d3.forceCollide<ForceNode>().radius((node) => nodeRadius(node) + 14),
      );
    return;
  }

  simulation
    .nodes(nodes)
    .force("link", simulationLinks)
    .force("center", d3.forceCenter(width / 2, height / 2))
    .force(
      "collision",
      d3.forceCollide<ForceNode>().radius((node) => nodeRadius(node) + 14),
    );

  if (topologyChanged) {
    simulation.alpha(0.55).restart();
  }
};

const requestGraphUpdate = () => {
  if (pendingRenderFrame) return;
  pendingRenderFrame = window.requestAnimationFrame(() => {
    pendingRenderFrame = 0;
    updateGraph();
  });
};

const updateGraph = () => {
  const svgElement = svgRef.value;
  const containerElement = containerRef.value;
  if (!svgElement || !containerElement) return;

  const measuredWidth = containerElement.clientWidth || svgElement.clientWidth;
  if (!measuredWidth) return;

  const width = Math.max(measuredWidth, 640);
  const height = props.height;
  initializeCanvas(svgElement, width, height);
  if (!linkGroup || !nodeGroup) return;

  currentDisplayGraph = buildDisplayGraph(props.nodes, props.edges);
  const topologyKey = createTopologyKey(width, height, currentDisplayGraph);
  const topologyChanged = topologyKey !== lastTopologyKey;
  lastTopologyKey = topologyKey;
  const existingForceNodes = simulation?.nodes() ?? [];
  const nodes = buildForceNodes(
    currentDisplayGraph.nodes,
    currentDisplayGraph.edges,
    existingForceNodes as ForceNode[],
    width,
    height,
    loadSavedNodePositions(),
  );
  const links = currentDisplayGraph.edges.map((edge) => ({
    ...edge,
  })) as ForceLink[];
  applyProcessTreeLayout(nodes, links, width, height);
  updateEmptyState(width, height, Boolean(nodes.length));

  const linkSelection = linkGroup
    .selectAll<SVGLineElement, ForceLink>("line")
    .data(links, (item) => item.id);
  linkSelection.exit().transition().duration(180).attr("opacity", 0).remove();
  const linkEnter = linkSelection.enter().append("line").attr("opacity", 0);
  linkEnter.transition().duration(180).attr("opacity", 1);
  const link = linkEnter
    .merge(linkSelection)
    .attr("stroke-width", (item) => linkStrokeWidth(item.kind))
    .attr("stroke", (item) => linkStrokeColor(item.kind));

  const drag = d3
    .drag<SVGGElement, ForceNode>()
    .on("start", (event, node) => {
      if (!event.active) simulation?.alphaTarget(0.25).restart();
      node.fx = node.x;
      node.fy = node.y;
    })
    .on("drag", (event, node) => {
      node.fx = event.x;
      node.fy = event.y;
    })
    .on("end", (event, node) => {
      if (!event.active) simulation?.alphaTarget(0);
      node.fx = event.x;
      node.fy = event.y;
      node.x = event.x;
      node.y = event.y;
      node.positionLocked = true;
      persistNodePosition(node);
    });

  const nodeSelection = nodeGroup
    .selectAll<SVGGElement, ForceNode>("g.execution-node")
    .data(nodes, (item) => item.id);
  nodeSelection.exit().transition().duration(180).style("opacity", 0).remove();
  const nodeEnter = nodeSelection
    .enter()
    .append("g")
    .attr("class", "execution-node")
    .style("opacity", 0);
  nodeEnter.append("circle");
  nodeEnter.append("text").style("pointer-events", "none");
  nodeEnter.append("title");
  nodeEnter.transition().duration(180).style("opacity", 1);

  const node = nodeEnter
    .merge(nodeSelection)
    .style("cursor", "pointer")
    .call(drag)
    .on("click", (_event, item) =>
      emit("select-node", item.metadata?.sourceNodeId || item.id),
    );

  node
    .select<SVGCircleElement>("circle")
    .attr("r", (item) => nodeRadius(item))
    .attr("fill", (item) => kindColor(item.kind))
    .attr("stroke", (item) =>
      item.id === props.selectedNodeId ||
      item.metadata?.sourceNodeId === props.selectedNodeId
        ? "#111827"
        : "#ffffff",
    )
    .attr("stroke-width", (item) =>
      item.id === props.selectedNodeId ||
      item.metadata?.sourceNodeId === props.selectedNodeId
        ? 3
        : 1.5,
    );

  node
    .select<SVGTextElement>("text")
    .text((item) => truncate(item.label))
    .attr("x", (item) => nodeRadius(item) + 6)
    .attr("y", 4)
    .attr("font-size", 11)
    .attr("fill", "#111827");

  node
    .select<SVGTitleElement>("title")
    .text(
      (item) =>
        `${item.kind}: ${item.label}${item.subtitle ? `\n${item.subtitle}` : ""}`,
    );

  updateSimulation(nodes, links, width, height, topologyChanged);

  if (!nodes.length) {
    simulation?.stop();
  }

  simulation?.on("tick", () => {
    link
      .attr("x1", (item) => (item.source as ForceNode).x ?? 0)
      .attr("y1", (item) => (item.source as ForceNode).y ?? 0)
      .attr("x2", (item) => (item.target as ForceNode).x ?? 0)
      .attr("y2", (item) => (item.target as ForceNode).y ?? 0);

    node.attr(
      "transform",
      (item) => `translate(${item.x ?? 0},${item.y ?? 0})`,
    );
  });
};

watch(
  () => [props.nodes, props.edges, props.selectedNodeId, props.height],
  () => requestGraphUpdate(),
  { deep: true },
);

onMounted(async () => {
  await nextTick();
  requestGraphUpdate();
  if (typeof ResizeObserver !== "undefined" && containerRef.value) {
    resizeObserver = new ResizeObserver(() => requestGraphUpdate());
    resizeObserver.observe(containerRef.value);
  }
});

onBeforeUnmount(() => {
  if (pendingRenderFrame) {
    window.cancelAnimationFrame(pendingRenderFrame);
    pendingRenderFrame = 0;
  }
  resizeObserver?.disconnect();
  resizeObserver = null;
  simulation?.stop();
});
</script>

<template>
  <div ref="containerRef" class="execution-graph-canvas">
    <svg ref="svgRef" class="execution-graph-svg" />
  </div>
</template>

<style scoped>
.execution-graph-canvas {
  width: 100%;
  height: 100%;
  min-height: 620px;
  border-radius: 12px;
  overflow: hidden;
  background:
    radial-gradient(
      circle at top left,
      rgba(59, 130, 246, 0.08),
      transparent 35%
    ),
    linear-gradient(
      180deg,
      rgba(248, 250, 252, 0.98),
      rgba(241, 245, 249, 0.98)
    );
  border: 1px solid #e2e8f0;
}

.execution-graph-svg {
  width: 100%;
  height: 100%;
  min-height: 620px;
  display: block;
}
</style>
