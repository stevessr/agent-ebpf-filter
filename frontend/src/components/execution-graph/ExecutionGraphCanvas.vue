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

interface DisplayGraph {
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
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
let rootGroup: d3.Selection<SVGGElement, unknown, null, undefined> | null = null;
let linkGroup: d3.Selection<SVGGElement, unknown, null, undefined> | null = null;
let nodeGroup: d3.Selection<SVGGElement, unknown, null, undefined> | null = null;
let emptyGroup: d3.Selection<SVGGElement, unknown, null, undefined> | null = null;
let zoomBehavior: d3.ZoomBehavior<SVGSVGElement, unknown> | null = null;
let lastTopologyKey = '';
let currentDisplayGraph: DisplayGraph = { nodes: [], edges: [] };

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
  const eventCount = Number(node.metadata?.eventCount ?? 1);
  const aggregateBoost = Number.isFinite(eventCount) && eventCount > 1 ? Math.min(8, Math.log2(eventCount) * 2) : 0;
  switch (node.kind) {
    case 'agent_run':
      return 18 + aggregateBoost;
    case 'tool_call':
      return 16 + aggregateBoost;
    case 'process':
      return 14 + aggregateBoost;
    case 'policy_alert':
    case 'policy_decision':
      return 12 + aggregateBoost;
    default:
      return 10 + aggregateBoost;
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

const processTreeEdgeKinds = new Set(['child_process', 'parent_process', 'exec_chain', 'spawned']);
const activityEdgeKinds = new Set(['observed', 'execed', 'waited', 'exited', 'reviewed', 'alerted']);

const linkStrokeWidth = (item: ForceLink) => (item.kind === 'alerted' || item.kind === 'blocked' ? 2.4 : 1.4);

const linkStrokeColor = (item: ForceLink) => {
  if (item.kind === 'alerted' || item.kind === 'blocked') return '#dc2626';
  if (item.kind === 'rewritten') return '#7c3aed';
  if (item.kind === 'child_process' || item.kind === 'parent_process') return '#059669';
  if (item.kind === 'exec_chain') return '#2563eb';
  return '#cbd5e1';
};

const linkDistance = (link: ForceLink) => {
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
};

const linkStrength = (link: ForceLink) => (processTreeEdgeKinds.has(link.kind) ? 0.85 : link.kind === 'contains' ? 0.55 : 0.35);

const createTopologyKey = () => [
  props.height,
  currentDisplayGraph.nodes.map((node) => `${node.id}:${node.kind}:${node.label}:${node.subtitle ?? ''}`).join(''),
  currentDisplayGraph.edges.map((edge) => `${edge.id}:${edge.source}:${edge.target}:${edge.kind}:${edge.label ?? ''}`).join(''),
].join('');

const processDisplayLabel = (node: ExecutionGraphNode | undefined) => {
  if (!node) return '';
  const pid = String(node.metadata?.pid ?? node.pid ?? '').trim();
  const label = /^pid \d+$/.test(node.label.trim()) ? 'process' : node.label;
  return pid ? `${label} (${pid})` : label;
};

const buildDisplayGraph = (): DisplayGraph => {
  const sourceNodes = new Map(props.nodes.map((node) => [node.id, node]));
  const eventToProcess = new Map<string, string>();
  props.edges.forEach((edge) => {
    if (!activityEdgeKinds.has(edge.kind)) return;
    const source = sourceNodes.get(edge.source);
    const target = sourceNodes.get(edge.target);
    if (source?.kind === 'process' && target && target.kind !== 'process') eventToProcess.set(target.id, source.id);
  });

  const aggregateByKey = new Map<string, { node: ExecutionGraphNode; eventIds: string[]; sourceIds: string[] }>();
  const aggregateIdByEventId = new Map<string, string>();
  const processToAggregateIds = new Map<string, string[]>();
  props.nodes.forEach((node) => {
    const processId = eventToProcess.get(node.id);
    if (!processId) return;
    const processNode = sourceNodes.get(processId);
    const processLabel = processDisplayLabel(processNode);
    const eventType = node.metadata?.type || node.label || node.kind;
    const key = `${processId}${node.kind}${eventType}`;
    const existing = aggregateByKey.get(key);
    if (existing) {
      existing.eventIds.push(node.id);
      existing.sourceIds.push(node.id);
      existing.node.riskScore = Math.max(existing.node.riskScore ?? 0, node.riskScore ?? 0);
      return;
    }
    const aggregateId = `agg:${processId}:${node.kind}:${eventType}`;
    const ids = processToAggregateIds.get(processId) ?? [];
    ids.push(aggregateId);
    processToAggregateIds.set(processId, ids);
    aggregateByKey.set(key, {
      eventIds: [node.id],
      sourceIds: [node.id],
      node: {
        ...node,
        id: aggregateId,
        label: processLabel ? `${processLabel} · ${eventType}` : eventType,
        subtitle: [processLabel, node.subtitle].filter(Boolean).join(' · '),
        metadata: {
          ...(node.metadata ?? {}),
          sourceNodeId: node.id,
          eventCount: '1',
        },
      },
    });
    aggregateIdByEventId.set(node.id, aggregateId);
  });

  aggregateByKey.forEach(({ node, eventIds, sourceIds }) => {
    eventIds.forEach((id) => aggregateIdByEventId.set(id, node.id));
    if (eventIds.length <= 1) return;
    node.label = `${node.label} ×${eventIds.length}`;
    node.subtitle = `${node.subtitle || 'events'} · ${eventIds.length} events`;
    node.metadata = {
      ...(node.metadata ?? {}),
      sourceNodeId: sourceIds[0],
      eventCount: String(eventIds.length),
    };
  });

  const displayNodes = props.nodes
    .filter((node) => node.kind !== 'process')
    .filter((node) => !aggregateIdByEventId.has(node.id))
    .concat([...aggregateByKey.values()].map((item) => item.node));
  const displayNodeIds = new Set(displayNodes.map((node) => node.id));
  const edgeById = new Map<string, ExecutionGraphEdge>();

  const representativeForProcess = (processId: string) => processToAggregateIds.get(processId)?.[0] ?? '';

  props.edges.forEach((edge) => {
    const rawSource = aggregateIdByEventId.get(edge.source) ?? (sourceNodes.get(edge.source)?.kind === 'process' ? representativeForProcess(edge.source) : edge.source);
    const rawTarget = aggregateIdByEventId.get(edge.target) ?? (sourceNodes.get(edge.target)?.kind === 'process' ? representativeForProcess(edge.target) : edge.target);
    const source = rawSource;
    const target = rawTarget;
    if (!source || !target || source === target) return;
    if (!displayNodeIds.has(source) || !displayNodeIds.has(target)) return;
    const id = `${source}->${target}:${edge.kind}`;
    if (!edgeById.has(id)) {
      edgeById.set(id, { ...edge, id, source, target });
    }
  });

  return { nodes: displayNodes, edges: [...edgeById.values()] };
};

const initializeCanvas = (svgElement: SVGSVGElement, width: number, height: number) => {
  const svg = d3.select(svgElement);
  svg.attr('viewBox', `0 0 ${width} ${height}`);
  if (rootGroup && linkGroup && nodeGroup && emptyGroup && zoomBehavior) return;

  rootGroup = svg.append('g');
  linkGroup = rootGroup.append('g').attr('stroke', '#cbd5e1').attr('stroke-opacity', 0.75);
  nodeGroup = rootGroup.append('g');
  emptyGroup = rootGroup.append('g');

  emptyGroup
    .append('text')
    .attr('text-anchor', 'middle')
    .attr('fill', '#64748b')
    .attr('font-size', 14)
    .text('No graph data');

  zoomBehavior = d3.zoom<SVGSVGElement, unknown>()
    .scaleExtent([0.35, 2.5])
    .on('zoom', (event) => {
      rootGroup?.attr('transform', event.transform.toString());
      persistZoom(event.transform);
    });
  svg.call(zoomBehavior);
  svg.call(zoomBehavior.transform, loadPersistedZoom());
};

const getNodePosition = (node: ExecutionGraphNode, existingById: Map<string, ForceNode>, width: number, height: number) => {
  const relatedEdge = currentDisplayGraph.edges.find((edge) => edge.source === node.id || edge.target === node.id);
  const relatedId = relatedEdge?.source === node.id ? relatedEdge.target : relatedEdge?.source;
  const relatedNode = relatedId ? existingById.get(relatedId) : undefined;
  return {
    x: relatedNode?.x ?? width / 2 + (Math.random() - 0.5) * 80,
    y: relatedNode?.y ?? height / 2 + (Math.random() - 0.5) * 80,
  };
};

const buildForceNodes = (width: number, height: number) => {
  const existingById = new Map((simulation?.nodes() ?? []).map((node) => [node.id, node]));
  return currentDisplayGraph.nodes.map((node) => {
    const existing = existingById.get(node.id);
    if (existing) {
      Object.assign(existing, node);
      return existing;
    }
    return { ...node, ...getNodePosition(node, existingById, width, height) } as ForceNode;
  });
};

const processSortValue = (node: ForceNode | undefined) => {
  const pid = Number(node?.metadata?.pid ?? node?.pid);
  if (Number.isFinite(pid)) return pid;
  return Number.MAX_SAFE_INTEGER;
};

const applyProcessTreeLayout = (nodes: ForceNode[], links: ForceLink[], width: number, height: number) => {
  nodes.forEach((node) => {
    node.fx = null;
    node.fy = null;
  });

  const processLinks = links.filter((link) => processTreeEdgeKinds.has(link.kind));
  if (!processLinks.length) return;

  const nodeById = new Map(nodes.map((node) => [node.id, node]));
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

  children.forEach((ids) => {
    ids.sort((left, right) => processSortValue(nodeById.get(left)) - processSortValue(nodeById.get(right)) || left.localeCompare(right));
  });

  const roots = [...processNodeIds]
    .filter((id) => !incoming.has(id))
    .sort((left, right) => processSortValue(nodeById.get(left)) - processSortValue(nodeById.get(right)) || left.localeCompare(right));
  if (!roots.length && processNodeIds.size) roots.push([...processNodeIds][0]);

  const levels = new Map<string, number>();
  const ySlots = new Map<string, number>();
  const visited = new Set<string>();
  let nextSlot = 0;

  const assignSubtree = (id: string, level: number): number => {
    if (visited.has(id)) return ySlots.get(id) ?? nextSlot;
    visited.add(id);
    levels.set(id, level);

    const childSlots = (children.get(id) ?? [])
      .filter((child) => processNodeIds.has(child))
      .map((child) => assignSubtree(child, level + 1));

    if (!childSlots.length) {
      const slot = nextSlot;
      nextSlot += 1;
      ySlots.set(id, slot);
      return slot;
    }

    const slot = (Math.min(...childSlots) + Math.max(...childSlots)) / 2;
    ySlots.set(id, slot);
    return slot;
  };

  roots.forEach((root) => assignSubtree(root, 0));
  [...processNodeIds]
    .filter((id) => !visited.has(id))
    .sort((left, right) => processSortValue(nodeById.get(left)) - processSortValue(nodeById.get(right)) || left.localeCompare(right))
    .forEach((id) => assignSubtree(id, 0));

  const maxLevel = Math.max(0, ...levels.values());
  const leftPadding = 96;
  const rightPadding = 180;
  const topPadding = 72;
  const bottomPadding = 72;
  const levelGap = Math.max(150, Math.min(260, (width - leftPadding - rightPadding) / Math.max(1, maxLevel)));
  const slotCount = Math.max(1, nextSlot);
  const rowGap = Math.max(64, Math.min(128, (height - topPadding - bottomPadding) / Math.max(1, slotCount - 1)));
  const totalTreeHeight = (slotCount - 1) * rowGap;
  const verticalOffset = Math.max(topPadding, (height - totalTreeHeight) / 2);

  processNodeIds.forEach((id) => {
    const node = nodeById.get(id);
    const level = levels.get(id);
    const slot = ySlots.get(id);
    if (!node || level === undefined || slot === undefined) return;
    node.fx = leftPadding + level * levelGap;
    node.fy = verticalOffset + slot * rowGap;
  });
};

const updateEmptyState = (width: number, height: number, hasNodes: boolean) => {
  if (!emptyGroup) return;
  emptyGroup.style('display', hasNodes ? 'none' : '');
  emptyGroup
    .select('text')
    .attr('x', width / 2)
    .attr('y', height / 2);
};

const updateSimulation = (nodes: ForceNode[], links: ForceLink[], width: number, height: number, topologyChanged: boolean) => {
  const simulationLinks = d3
    .forceLink<ForceNode, ForceLink>(links)
    .id((node) => node.id)
    .distance(linkDistance)
    .strength(linkStrength);

  if (!simulation) {
    simulation = d3
      .forceSimulation<ForceNode>(nodes)
      .force('link', simulationLinks)
      .force('charge', d3.forceManyBody<ForceNode>().strength(-340))
      .force('center', d3.forceCenter(width / 2, height / 2))
      .force('collision', d3.forceCollide<ForceNode>().radius((node) => nodeRadius(node) + 14));
    return;
  }

  simulation
    .nodes(nodes)
    .force('link', simulationLinks)
    .force('center', d3.forceCenter(width / 2, height / 2))
    .force('collision', d3.forceCollide<ForceNode>().radius((node) => nodeRadius(node) + 14));

  if (topologyChanged) {
    simulation.alpha(0.35).restart();
  }
};

const updateGraph = () => {
  const svgElement = svgRef.value;
  if (!svgElement) return;

  const width = Math.max(svgElement.clientWidth || 960, 640);
  const height = props.height;
  initializeCanvas(svgElement, width, height);
  if (!linkGroup || !nodeGroup) return;

  currentDisplayGraph = buildDisplayGraph();
  const topologyKey = createTopologyKey();
  const topologyChanged = topologyKey !== lastTopologyKey;
  lastTopologyKey = topologyKey;
  const nodes = buildForceNodes(width, height);
  const links = currentDisplayGraph.edges.map((edge) => ({ ...edge })) as ForceLink[];
  applyProcessTreeLayout(nodes, links, width, height);
  updateEmptyState(width, height, Boolean(nodes.length));

  const linkSelection = linkGroup
    .selectAll<SVGLineElement, ForceLink>('line')
    .data(links, (item) => item.id);
  linkSelection.exit().transition().duration(180).attr('opacity', 0).remove();
  const linkEnter = linkSelection.enter().append('line').attr('opacity', 0);
  linkEnter.transition().duration(180).attr('opacity', 1);
  const link = linkEnter
    .merge(linkSelection)
    .attr('stroke-width', linkStrokeWidth)
    .attr('stroke', linkStrokeColor);

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

  const nodeSelection = nodeGroup
    .selectAll<SVGGElement, ForceNode>('g.execution-node')
    .data(nodes, (item) => item.id);
  nodeSelection.exit().transition().duration(180).style('opacity', 0).remove();
  const nodeEnter = nodeSelection
    .enter()
    .append('g')
    .attr('class', 'execution-node')
    .style('opacity', 0);
  nodeEnter.append('circle');
  nodeEnter.append('text').style('pointer-events', 'none');
  nodeEnter.append('title');
  nodeEnter.transition().duration(180).style('opacity', 1);

  const node = nodeEnter
    .merge(nodeSelection)
    .style('cursor', 'pointer')
    .call(drag)
    .on('click', (_event, item) => emit('select-node', item.metadata?.sourceNodeId || item.id));

  node
    .select<SVGCircleElement>('circle')
    .attr('r', (item) => nodeRadius(item))
    .attr('fill', (item) => kindColor(item.kind))
    .attr('stroke', (item) => (item.id === props.selectedNodeId || item.metadata?.sourceNodeId === props.selectedNodeId ? '#111827' : '#ffffff'))
    .attr('stroke-width', (item) => (item.id === props.selectedNodeId || item.metadata?.sourceNodeId === props.selectedNodeId ? 3 : 1.5));

  node
    .select<SVGTextElement>('text')
    .text((item) => truncate(item.label))
    .attr('x', (item) => nodeRadius(item) + 6)
    .attr('y', 4)
    .attr('font-size', 11)
    .attr('fill', '#111827');

  node
    .select<SVGTitleElement>('title')
    .text((item) => `${item.kind}: ${item.label}${item.subtitle ? `\n${item.subtitle}` : ''}`);

  updateSimulation(nodes, links, width, height, topologyChanged);

  if (!nodes.length) {
    simulation?.stop();
  }

  simulation?.on('tick', () => {
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
  () => updateGraph(),
  { deep: true },
);

onMounted(() => updateGraph());

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
