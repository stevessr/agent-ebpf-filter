import type * as d3 from 'd3';
import type { ExecutionGraphEdge, ExecutionGraphNode } from '../../types/executionGraph';
import {
  activityEdgeKinds,
  processDisplayLabel,
  processSortValue,
  processTreeEdgeKinds,
} from './useExecutionGraphHelpers';

export interface ForceNode extends d3.SimulationNodeDatum, ExecutionGraphNode {}
export interface ForceLink extends d3.SimulationLinkDatum<ForceNode> {
  id: string;
  kind: string;
  label?: string;
  source: string | ForceNode;
  target: string | ForceNode;
}

export interface DisplayGraph {
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
}

const processPid = (node: ExecutionGraphNode) => {
  if (node.kind !== 'process') return '';
  const explicitPid = String(node.metadata?.pid ?? node.pid ?? '').trim();
  if (explicitPid) return explicitPid;
  const idMatch = /^proc:(\d+)$/.exec(node.id.trim());
  if (idMatch) return idMatch[1];
  const labelMatch = /^pid\s+(\d+)$/i.exec(node.label.trim());
  return labelMatch?.[1] ?? '';
};

const isPidOnlyProcessNode = (node: ExecutionGraphNode) => /^pid\s+\d+$/i.test(node.label.trim());

const formatProcessNode = (node: ExecutionGraphNode): ExecutionGraphNode => {
  const pid = processPid(node);
  const metadata = { ...(node.metadata ?? {}) };
  if (pid) metadata.pid = pid;
  return {
    ...node,
    label: processDisplayLabel({ ...node, metadata }) || node.label,
    subtitle: node.subtitle || (pid ? `pid=${pid}` : node.subtitle),
    metadata,
  };
};

const mergeProcessNode = (current: ExecutionGraphNode | undefined, next: ExecutionGraphNode): ExecutionGraphNode => {
  if (!current) return formatProcessNode({ ...next, metadata: next.metadata ? { ...next.metadata } : undefined });
  const currentPidOnly = isPidOnlyProcessNode(current);
  const nextPidOnly = isPidOnlyProcessNode(next);
  const winner = currentPidOnly && !nextPidOnly ? next : current;
  const fallback = winner === current ? next : current;
  const pid = processPid(winner) || processPid(fallback);
  const metadata = { ...(fallback.metadata ?? {}), ...(winner.metadata ?? {}) };
  if (pid) metadata.pid = pid;
  const subtitleParts = [winner.subtitle, fallback.subtitle]
    .map((value) => value?.trim())
    .filter((value, index, values): value is string => Boolean(value) && values.indexOf(value) === index);
  const label = processDisplayLabel({ ...winner, metadata, pid: winner.pid || fallback.pid }) || winner.label;
  return {
    ...fallback,
    ...winner,
    id: winner.id,
    label,
    subtitle: subtitleParts[0] ?? (pid ? `pid=${pid}` : winner.subtitle),
    pid: winner.pid || fallback.pid,
    riskScore: Math.max(winner.riskScore ?? 0, fallback.riskScore ?? 0) || undefined,
    metadata,
  };
};

const normalizeProcessNodes = (
  nodes: ExecutionGraphNode[],
  edges: ExecutionGraphEdge[],
): DisplayGraph => {
  const processByPid = new Map<string, ExecutionGraphNode>();
  const canonicalIdBySourceId = new Map<string, string>();
  const normalizedNodes: ExecutionGraphNode[] = [];

  nodes.forEach((node) => {
    const pid = processPid(node);
    if (!pid) {
      normalizedNodes.push(node);
      return;
    }
    processByPid.set(pid, mergeProcessNode(processByPid.get(pid), node));
  });

  processByPid.forEach((node, pid) => {
    canonicalIdBySourceId.set(`proc:${pid}`, node.id);
    nodes.forEach((candidate) => {
      if (processPid(candidate) === pid) canonicalIdBySourceId.set(candidate.id, node.id);
    });
    normalizedNodes.push(node);
  });

  const normalizedEdges = edges.map((edge) => {
    const source = canonicalIdBySourceId.get(edge.source) ?? edge.source;
    const target = canonicalIdBySourceId.get(edge.target) ?? edge.target;
    return source === edge.source && target === edge.target
      ? edge
      : { ...edge, id: `${source}->${target}:${edge.kind}`, source, target };
  });

  return { nodes: normalizedNodes, edges: normalizedEdges };
};

/**
 * Build the aggregated display graph from raw nodes and edges.
 * Groups events by process and type, merges duplicate edges.
 */
export const buildDisplayGraph = (
  rawNodes: ExecutionGraphNode[],
  rawEdges: ExecutionGraphEdge[],
): DisplayGraph => {
  const normalizedGraph = normalizeProcessNodes(rawNodes, rawEdges);
  const sourceNodes = new Map(normalizedGraph.nodes.map((node) => [node.id, node]));
  const eventToProcess = new Map<string, string>();
  normalizedGraph.edges.forEach((edge) => {
    if (!activityEdgeKinds.has(edge.kind)) return;
    const source = sourceNodes.get(edge.source);
    const target = sourceNodes.get(edge.target);
    if (source?.kind === 'process' && target && target.kind !== 'process')
      eventToProcess.set(target.id, source.id);
  });

  const aggregateByKey = new Map<
    string,
    { node: ExecutionGraphNode; eventIds: string[]; sourceIds: string[] }
  >();
  const aggregateIdByEventId = new Map<string, string>();
  normalizedGraph.nodes.forEach((node) => {
    const processId = eventToProcess.get(node.id);
    if (!processId) return;
    const processNode = sourceNodes.get(processId);
    const pLabel = processDisplayLabel(processNode);
    const eventType = node.metadata?.type || node.label || node.kind;
    const key = `${processId}${node.kind}${eventType}`;
    const existing = aggregateByKey.get(key);
    if (existing) {
      existing.eventIds.push(node.id);
      existing.sourceIds.push(node.id);
      existing.node.riskScore = Math.max(existing.node.riskScore ?? 0, node.riskScore ?? 0);
      return;
    }
    const aggregateId = `agg:${processId}:${node.kind}:${eventType}`;
    aggregateByKey.set(key, {
      eventIds: [node.id],
      sourceIds: [node.id],
      node: {
        ...node,
        id: aggregateId,
        label: pLabel ? `${pLabel} · ${eventType}` : eventType,
        subtitle: [pLabel, node.subtitle].filter(Boolean).join(' · '),
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

  const displayNodes = normalizedGraph.nodes
    .filter((node) => node.kind === 'process' || !aggregateIdByEventId.has(node.id))
    .concat([...aggregateByKey.values()].map((item) => item.node));
  const displayNodeIds = new Set(displayNodes.map((node) => node.id));
  const edgeById = new Map<string, ExecutionGraphEdge>();

  normalizedGraph.edges.forEach((edge) => {
    const source = aggregateIdByEventId.get(edge.source) ?? edge.source;
    const target = aggregateIdByEventId.get(edge.target) ?? edge.target;
    if (!source || !target || source === target) return;
    if (!displayNodeIds.has(source) || !displayNodeIds.has(target)) return;
    const id = `${source}->${target}:${edge.kind}`;
    if (!edgeById.has(id)) {
      edgeById.set(id, { ...edge, id, source, target });
    }
  });

  return { nodes: displayNodes, edges: [...edgeById.values()] };
};

/**
 * Build force simulation nodes, reusing existing positions when available.
 */
export const buildForceNodes = (
  displayNodes: ExecutionGraphNode[],
  displayEdges: ExecutionGraphEdge[],
  existingNodes: ForceNode[],
  width: number,
  height: number,
): ForceNode[] => {
  const existingById = new Map(existingNodes.map((node) => [node.id, node]));
  return displayNodes.map((node) => {
    const existing = existingById.get(node.id);
    if (existing) {
      Object.assign(existing, node);
      return existing;
    }
    const relatedEdge = displayEdges.find(
      (edge) => edge.source === node.id || edge.target === node.id,
    );
    const relatedId =
      relatedEdge?.source === node.id ? relatedEdge.target : relatedEdge?.source;
    const relatedNode = relatedId ? existingById.get(relatedId) : undefined;
    return {
      ...node,
      x: relatedNode?.x ?? width / 2 + (Math.random() - 0.5) * 80,
      y: relatedNode?.y ?? height / 2 + (Math.random() - 0.5) * 80,
    } as ForceNode;
  });
};

/**
 * Apply hierarchical tree layout to process nodes based on parent-child links.
 */
export const applyProcessTreeLayout = (
  nodes: ForceNode[],
  links: ForceLink[],
  width: number,
  height: number,
) => {
  nodes.forEach((node) => {
    node.fx = null;
    node.fy = null;
  });

  const processLinks = links.filter((link) =>
    processTreeEdgeKinds.has(link.kind),
  );
  if (!processLinks.length) return;

  const nodeById = new Map(nodes.map((node) => [node.id, node]));
  const processNodeIds = new Set(
    nodes.filter((node) => node.kind === 'process').map((node) => node.id),
  );
  const children = new Map<string, string[]>();
  const incoming = new Set<string>();
  processLinks.forEach((link) => {
    const source = String(link.source);
    const target = String(link.target);
    if (!processNodeIds.has(source) || !processNodeIds.has(target) || source === target)
      return;
    const list = children.get(source) ?? [];
    if (!list.includes(target)) list.push(target);
    children.set(source, list);
    incoming.add(target);
  });

  children.forEach((ids) => {
    ids.sort(
      (left, right) =>
        processSortValue(nodeById.get(left)?.pid) -
          processSortValue(nodeById.get(right)?.pid) ||
        left.localeCompare(right),
    );
  });

  const roots = [...processNodeIds]
    .filter((id) => !incoming.has(id))
    .sort(
      (left, right) =>
        processSortValue(nodeById.get(left)?.pid) -
          processSortValue(nodeById.get(right)?.pid) ||
        left.localeCompare(right),
    );
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
    .sort(
      (left, right) =>
        processSortValue(nodeById.get(left)?.pid) -
          processSortValue(nodeById.get(right)?.pid) ||
        left.localeCompare(right),
    )
    .forEach((id) => assignSubtree(id, 0));

  const maxLevel = Math.max(0, ...levels.values());
  const leftPadding = 96;
  const rightPadding = 180;
  const topPadding = 72;
  const bottomPadding = 72;
  const levelGap = Math.max(
    150,
    Math.min(260, (width - leftPadding - rightPadding) / Math.max(1, maxLevel)),
  );
  const slotCount = Math.max(1, nextSlot);
  const rowGap = Math.max(
    64,
    Math.min(128, (height - topPadding - bottomPadding) / Math.max(1, slotCount - 1)),
  );
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

/**
 * Create a string key that captures the current topology for change detection.
 */
export const createTopologyKey = (
  width: number,
  height: number,
  displayGraph: DisplayGraph,
) =>
  [
    width,
    height,
    displayGraph.nodes
      .map((node) => `${node.id}:${node.kind}:${node.label}:${node.subtitle ?? ''}`)
      .join(''),
    displayGraph.edges
      .map(
        (edge) =>
          `${edge.id}:${edge.source}:${edge.target}:${edge.kind}:${edge.label ?? ''}`,
      )
      .join(''),
  ].join('');
