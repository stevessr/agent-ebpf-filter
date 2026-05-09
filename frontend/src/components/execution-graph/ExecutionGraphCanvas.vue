<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import * as d3 from 'd3';

import type { ExecutionGraphEdge, ExecutionGraphNode } from '../../types/executionGraph';

interface ForceNode extends d3.SimulationNodeDatum, ExecutionGraphNode {}
interface ForceLink extends d3.SimulationLinkDatum<ForceNode> {
  id: string;
  kind: string;
  label?: string;
  source: string | ForceNode;
  target: string | ForceNode;
}

const props = withDefaults(defineProps<{
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
  selectedNodeId?: string;
  height?: number;
  zoomStorageKey?: string;
}>(), {
  selectedNodeId: '',
  height: 620,
  zoomStorageKey: 'agent-ebpf.execution-graph.zoom',
});

const emit = defineEmits<{
  (event: 'select-node', nodeId: string): void;
}>();

const svgRef = ref<SVGSVGElement | null>(null);
let simulation: d3.Simulation<ForceNode, ForceLink> | null = null;

const kindColor = (kind: string) => {
  const colorMap: Record<string, string> = {
    agent_run: '#7c3aed',
    tool_call: '#2563eb',
    process: '#10b981',
    syscall: '#f59e0b',
    wrapper_event: '#0891b2',
    hook_event: '#0f766e',
    file: '#64748b',
    network: '#ef4444',
    policy_decision: '#111827',
    policy_alert: '#dc2626',
    exit_status: '#6b7280',
  };
  return colorMap[kind] ?? '#94a3b8';
};

const nodeRadius = (node: ExecutionGraphNode) => {
  switch (node.kind) {
    case 'agent_run':
      return 18;
    case 'tool_call':
      return 16;
    case 'process':
      return 14;
    case 'policy_alert':
    case 'policy_decision':
      return 12;
    default:
      return 10;
  }
};

const truncate = (value: string, max = 28) => {
  if (value.length <= max) return value;
  return `${value.slice(0, max - 1)}…`;
};

const loadPersistedZoom = () => {
  if (!props.zoomStorageKey) return d3.zoomIdentity;
  try {
    const raw = localStorage.getItem(props.zoomStorageKey);
    if (!raw) return d3.zoomIdentity;
    const parsed = JSON.parse(raw) as { x?: unknown; y?: unknown; k?: unknown };
    const x = Number(parsed.x);
    const y = Number(parsed.y);
    const k = Number(parsed.k);
    if (!Number.isFinite(x) || !Number.isFinite(y) || !Number.isFinite(k) || k <= 0) {
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
    localStorage.setItem(props.zoomStorageKey, JSON.stringify({ x: transform.x, y: transform.y, k: transform.k }));
  } catch {
    // Ignore storage quota / privacy mode failures. The graph remains usable.
  }
};

const renderGraph = () => {
  const svgElement = svgRef.value;
  if (!svgElement) return;

  simulation?.stop();

  const width = Math.max(svgElement.clientWidth || 960, 640);
  const height = props.height;
  const svg = d3.select(svgElement);
  svg.selectAll('*').remove();
  svg.attr('viewBox', `0 0 ${width} ${height}`);

  const root = svg.append('g');
  const zoomBehavior = d3.zoom<SVGSVGElement, unknown>()
    .scaleExtent([0.35, 2.5])
    .on('zoom', (event) => {
      root.attr('transform', event.transform.toString());
      persistZoom(event.transform);
    });
  svg.call(zoomBehavior);
  svg.call(zoomBehavior.transform, loadPersistedZoom());

  const nodes = props.nodes.map((node) => ({ ...node })) as ForceNode[];
  const links = props.edges.map((edge) => ({ ...edge })) as ForceLink[];

  if (!nodes.length) {
    root
      .append('text')
      .attr('x', width / 2)
      .attr('y', height / 2)
      .attr('text-anchor', 'middle')
      .attr('fill', '#64748b')
      .attr('font-size', 14)
      .text('No graph data');
    return;
  }

  const processTreeEdgeKinds = new Set(['child_process', 'parent_process', 'exec_chain', 'spawned']);
  const processLinks = links.filter((link) => processTreeEdgeKinds.has(link.kind));
  if (processLinks.length) {
    const processNodeIds = new Set(nodes.filter((node) => node.kind === 'process').map((node) => node.id));
    const children = new Map<string, string[]>();
    const incoming = new Set<string>();
    processLinks.forEach((link) => {
      const source = String(link.source);
      const target = String(link.target);
      if (!processNodeIds.has(source) || !processNodeIds.has(target) || source === target) return;
      const list = children.get(source) ?? [];
      if (!list.includes(target)) list.push(target);
      children.set(source, list);
      incoming.add(target);
    });
    const roots = [...processNodeIds].filter((id) => !incoming.has(id));
    if (!roots.length && processNodeIds.size) roots.push([...processNodeIds][0]);
    const levels = new Map<string, number>();
    const queue = roots.map((id) => ({ id, level: 0 }));
    roots.forEach((id) => levels.set(id, 0));
    while (queue.length) {
      const current = queue.shift()!;
      for (const child of children.get(current.id) ?? []) {
        if (levels.has(child)) continue;
        levels.set(child, current.level + 1);
        queue.push({ id: child, level: current.level + 1 });
      }
    }
    for (const id of processNodeIds) {
      if (!levels.has(id)) levels.set(id, 0);
    }
    const byLevel = new Map<number, string[]>();
    levels.forEach((level, id) => {
      const list = byLevel.get(level) ?? [];
      list.push(id);
      byLevel.set(level, list);
    });
    const leftPadding = 90;
    const topPadding = 80;
    const levelGap = Math.max(150, Math.min(240, (width - leftPadding * 2) / Math.max(1, byLevel.size)));
    byLevel.forEach((ids, level) => {
      ids.sort();
      const rowGap = Math.max(70, Math.min(130, (height - topPadding * 2) / Math.max(1, ids.length)));
      ids.forEach((id, index) => {
        const node = nodes.find((item) => item.id === id);
        if (!node) return;
        node.fx = leftPadding + level * levelGap;
        node.fy = topPadding + index * rowGap;
      });
    });
  }

  const simulationLinks = d3
    .forceLink<ForceNode, ForceLink>(links)
    .id((node) => node.id)
    .distance((link) => {
      switch (link.kind) {
        case 'contains':
        case 'owns':
          return 95;
        case 'child_process':
        case 'parent_process':
        case 'exec_chain':
          return 80;
        case 'spawned':
        case 'waited':
          return 88;
        case 'connected':
        case 'opened':
        case 'read':
        case 'wrote':
        case 'deleted':
          return 110;
        default:
          return 120;
      }
    })
    .strength((link) => (processTreeEdgeKinds.has(link.kind) ? 0.85 : link.kind === 'contains' ? 0.55 : 0.35));

  simulation = d3
    .forceSimulation<ForceNode>(nodes)
    .force('link', simulationLinks)
    .force('charge', d3.forceManyBody<ForceNode>().strength(-340))
    .force('center', d3.forceCenter(width / 2, height / 2))
    .force('collision', d3.forceCollide<ForceNode>().radius((node) => nodeRadius(node) + 14));

  const link = root
    .append('g')
    .attr('stroke', '#cbd5e1')
    .attr('stroke-opacity', 0.75)
    .selectAll<SVGLineElement, ForceLink>('line')
    .data(links)
    .join('line')
    .attr('stroke-width', (item) => (item.kind === 'alerted' || item.kind === 'blocked' ? 2.4 : 1.4))
    .attr('stroke', (item) => {
      if (item.kind === 'alerted' || item.kind === 'blocked') return '#dc2626';
      if (item.kind === 'rewritten') return '#7c3aed';
      if (item.kind === 'child_process' || item.kind === 'parent_process') return '#059669';
      if (item.kind === 'exec_chain') return '#2563eb';
      return '#cbd5e1';
    });

  const drag = d3.drag<SVGGElement, ForceNode>()
    .on('start', (event, node) => {
      if (!event.active) simulation?.alphaTarget(0.25).restart();
      node.fx = node.x;
      node.fy = node.y;
    })
    .on('drag', (event, node) => {
      node.fx = event.x;
      node.fy = event.y;
    })
    .on('end', (event, node) => {
      if (!event.active) simulation?.alphaTarget(0);
      node.fx = null;
      node.fy = null;
    });

  const node = root
    .append('g')
    .selectAll<SVGGElement, ForceNode>('g')
    .data(nodes)
    .join('g')
    .style('cursor', 'pointer')
    .call(drag)
    .on('click', (_event, item) => emit('select-node', item.id));

  node
    .append('circle')
    .attr('r', (item) => nodeRadius(item))
    .attr('fill', (item) => kindColor(item.kind))
    .attr('stroke', (item) => (item.id === props.selectedNodeId ? '#111827' : '#ffffff'))
    .attr('stroke-width', (item) => (item.id === props.selectedNodeId ? 3 : 1.5));

  node
    .append('text')
    .text((item) => truncate(item.label))
    .attr('x', (item) => nodeRadius(item) + 6)
    .attr('y', 4)
    .attr('font-size', 11)
    .attr('fill', '#111827')
    .style('pointer-events', 'none');

  node
    .append('title')
    .text((item) => `${item.kind}: ${item.label}${item.subtitle ? `\n${item.subtitle}` : ''}`);

  simulation.on('tick', () => {
    link
      .attr('x1', (item) => (item.source as ForceNode).x ?? 0)
      .attr('y1', (item) => (item.source as ForceNode).y ?? 0)
      .attr('x2', (item) => (item.target as ForceNode).x ?? 0)
      .attr('y2', (item) => (item.target as ForceNode).y ?? 0);

    node.attr('transform', (item) => `translate(${item.x ?? 0},${item.y ?? 0})`);
  });
};

watch(
  () => [props.nodes, props.edges, props.selectedNodeId, props.height],
  () => renderGraph(),
  { deep: true },
);

onMounted(() => renderGraph());

onBeforeUnmount(() => {
  simulation?.stop();
});
</script>

<template>
  <div class="execution-graph-canvas">
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
    radial-gradient(circle at top left, rgba(59, 130, 246, 0.08), transparent 35%),
    linear-gradient(180deg, rgba(248, 250, 252, 0.98), rgba(241, 245, 249, 0.98));
  border: 1px solid #e2e8f0;
}

.execution-graph-svg {
  width: 100%;
  height: 100%;
  min-height: 620px;
  display: block;
}
</style>
