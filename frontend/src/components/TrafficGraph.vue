<script setup lang="ts">
import * as d3 from 'd3';
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';

interface TrafficInterface {
  name: string;
  readSpeed: number;
  writeSpeed: number;
}

interface GraphNode {
  id: string;
  kind: 'internet' | 'interface';
  readSpeed: number;
  writeSpeed: number;
  totalSpeed: number;
  x: number;
  y: number;
}

interface GraphLink {
  id: string;
  source: string;
  target: string;
  speed: number;
}

interface LinkGeometry {
  x1: number;
  y1: number;
  c1x: number;
  c1y: number;
  c2x: number;
  c2y: number;
  x2: number;
  y2: number;
}

const props = withDefaults(defineProps<{
  interfaces: TrafficInterface[];
  height?: number;
}>(), {
  height: 420
});

const emit = defineEmits<{
  (event: 'select-interface', name: string): void;
}>();

const containerRef = ref<HTMLElement | null>(null);
const svgRef = ref<SVGSVGElement | null>(null);

let resizeObserver: ResizeObserver | null = null;
let dragDepth = 0;
let renderQueued = false;
const clickBlockUntil = new Map<string, number>();
const nodePositionCache = new Map<string, { x: number; y: number }>();

const megabyte = 1024 * 1024;
const highTraffic = 10 * megabyte;

const formatBytes = (bytes: number, decimals = 1) => {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const base = 1024;
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(base)), units.length - 1);
  return `${(bytes / Math.pow(base, index)).toFixed(index === 0 ? 0 : decimals)} ${units[index]}`;
};

const trafficColor = (speed: number) => {
  if (speed >= highTraffic) return '#ff4d4f';
  if (speed >= megabyte) return '#faad14';
  return '#52c41a';
};

const nodeColor = (speed: number) => {
  if (speed <= 0) return '#94a3b8';
  return trafficColor(speed);
};

const logGrowth = (value: number, min: number, max: number, ceiling: number) => {
  if (ceiling <= 0) return min;
  const normalized = Math.log1p(Math.max(0, value)) / Math.log1p(Math.max(ceiling, 1));
  return min + (max - min) * Math.min(1, normalized);
};

const getCubicPoint = (geometry: LinkGeometry, t: number) => {
  const mt = 1 - t;
  const mt2 = mt * mt;
  const t2 = t * t;
  const x = mt2 * mt * geometry.x1
    + 3 * mt2 * t * geometry.c1x
    + 3 * mt * t2 * geometry.c2x
    + t2 * t * geometry.x2;
  const y = mt2 * mt * geometry.y1
    + 3 * mt2 * t * geometry.c1y
    + 3 * mt * t2 * geometry.c2y
    + t2 * t * geometry.y2;
  return { x, y };
};

const getCubicTangent = (geometry: LinkGeometry, t: number) => {
  const mt = 1 - t;
  const x = 3 * mt * mt * (geometry.c1x - geometry.x1)
    + 6 * mt * t * (geometry.c2x - geometry.c1x)
    + 3 * t * t * (geometry.x2 - geometry.c2x);
  const y = 3 * mt * mt * (geometry.c1y - geometry.y1)
    + 6 * mt * t * (geometry.c2y - geometry.c1y)
    + 3 * t * t * (geometry.y2 - geometry.c2y);
  return { x, y };
};

const layoutInterfaces = (interfaces: TrafficInterface[]) => [...interfaces]
  .map((item) => ({
    ...item,
    totalSpeed: item.readSpeed + item.writeSpeed,
  }))
  .sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }));

const renderGraph = () => {
  if (!containerRef.value || !svgRef.value) return;

  const width = Math.max(containerRef.value.clientWidth, 320);
  const height = 420;
  const centerX = width / 2;
  const centerY = height / 2;
  const svg = d3.select(svgRef.value);

  svg.selectAll('*').remove();
  svg.attr('viewBox', `0 0 ${width} ${height}`);

  const interfaces = layoutInterfaces(props.interfaces);

  if (!interfaces.length) {
    svg
      .append('text')
      .attr('x', centerX)
      .attr('y', centerY - 6)
      .attr('text-anchor', 'middle')
      .attr('fill', '#64748b')
      .attr('font-size', 16)
      .attr('font-weight', 600)
      .text('No network interfaces detected');

    svg
      .append('text')
      .attr('x', centerX)
      .attr('y', centerY + 18)
      .attr('text-anchor', 'middle')
      .attr('fill', '#94a3b8')
      .attr('font-size', 12)
      .text('Waiting for network counters from /ws/system');
    return;
  }

  const aggregateIn = interfaces.reduce((sum, item) => sum + item.readSpeed, 0);
  const aggregateOut = interfaces.reduce((sum, item) => sum + item.writeSpeed, 0);

  const maxSpeed = Math.max(1, ...interfaces.map((item) => item.totalSpeed), aggregateIn, aggregateOut);
  const minDimension = Math.min(width, height);
  const nodeRadius = (speed: number) => logGrowth(speed, 22, 58, maxSpeed);
  const linkWidth = (speed: number) => logGrowth(speed, 1.4, 10, maxSpeed);
  const orbitRadius = Math.min(
    Math.max(minDimension * 0.24, 90),
    Math.max(minDimension / 2 - 92, 72),
  );
  const internetRadius = logGrowth(aggregateIn + aggregateOut, 58, 76, maxSpeed);

  const clampPosition = (x: number, y: number, radius: number) => {
    const minX = radius + 12;
    const maxX = Math.max(minX, width - radius - 12);
    const minY = radius + 12;
    const maxY = Math.max(minY, height - radius - 12);
    return {
      x: Math.min(maxX, Math.max(minX, x)),
      y: Math.min(maxY, Math.max(minY, y)),
    };
  };

  const activeNames = new Set(['Internet', ...interfaces.map((item) => item.name)]);
  [...nodePositionCache.keys()].forEach((name) => {
    if (!activeNames.has(name)) {
      nodePositionCache.delete(name);
    }
  });

  const internetCache = nodePositionCache.get('Internet');
  const internetPosition = internetCache
    ? clampPosition(internetCache.x, internetCache.y, internetRadius)
    : { x: centerX, y: centerY };
  if (internetCache && (internetCache.x !== internetPosition.x || internetCache.y !== internetPosition.y)) {
    nodePositionCache.set('Internet', internetPosition);
  }

  const nodes: GraphNode[] = [
    {
      id: 'Internet',
      kind: 'internet',
      readSpeed: aggregateIn,
      writeSpeed: aggregateOut,
      totalSpeed: aggregateIn + aggregateOut,
      x: internetPosition.x,
      y: internetPosition.y,
    },
    ...interfaces.map((item, index) => {
      const angle = Math.PI + (index / interfaces.length) * Math.PI * 2;
      const currentRadius = nodeRadius(item.totalSpeed);
      const maxRadius = Math.max(orbitRadius, minDimension / 2 - currentRadius - 20);
      const sizeBoost = Math.max(0, currentRadius - 22) * Math.max(2.2, minDimension / 120);
      const defaultRadius = Math.min(orbitRadius + sizeBoost, maxRadius);
      const defaultPosition = clampPosition(
        internetPosition.x + Math.cos(angle) * defaultRadius,
        internetPosition.y + Math.sin(angle) * defaultRadius,
        currentRadius,
      );
      const cachedPosition = nodePositionCache.get(item.name);
      const position = cachedPosition
        ? clampPosition(cachedPosition.x, cachedPosition.y, currentRadius)
        : defaultPosition;
      if (cachedPosition && (cachedPosition.x !== position.x || cachedPosition.y !== position.y)) {
        nodePositionCache.set(item.name, position);
      }
      return {
        id: item.name,
        kind: 'interface' as const,
        readSpeed: item.readSpeed,
        writeSpeed: item.writeSpeed,
        totalSpeed: item.totalSpeed,
        x: position.x,
        y: position.y,
      };
    }),
  ];

  const nodeById = new Map(nodes.map((node) => [node.id, node]));

  const links: GraphLink[] = [];
  interfaces.forEach((item) => {
    if (item.writeSpeed > 0) {
      links.push({
        id: `${item.name}-tx`,
        source: item.name,
        target: 'Internet',
        speed: item.writeSpeed,
      });
    }
    if (item.readSpeed > 0) {
      links.push({
        id: `${item.name}-rx`,
        source: 'Internet',
        target: item.name,
        speed: item.readSpeed,
      });
    }
  });

  const orbitSelection = svg
    .append('circle')
    .attr('cx', internetPosition.x)
    .attr('cy', internetPosition.y)
    .attr('r', orbitRadius)
    .attr('fill', 'none')
    .attr('stroke', 'rgba(148, 163, 184, 0.25)')
    .attr('stroke-dasharray', '6 10');

  const getNodeRadius = (node: GraphNode) => (node.kind === 'internet' ? internetRadius : nodeRadius(node.totalSpeed));

  const hubX = () => nodeById.get('Internet')?.x ?? internetPosition.x;
  const hubY = () => nodeById.get('Internet')?.y ?? internetPosition.y;

  const maskId = `traffic-link-mask-${Math.random().toString(36).slice(2, 10)}`;
  const defs = svg.append('defs');
  const linkMask = defs
    .append('mask')
    .attr('id', maskId)
    .attr('maskUnits', 'userSpaceOnUse')
    .attr('maskContentUnits', 'userSpaceOnUse')
    .attr('x', 0)
    .attr('y', 0)
    .attr('width', width)
    .attr('height', height);

  linkMask
    .append('rect')
    .attr('x', 0)
    .attr('y', 0)
    .attr('width', width)
    .attr('height', height)
    .attr('fill', '#fff');

  const linkMaskNodes = linkMask
    .append('g')
    .selectAll<SVGCircleElement, GraphNode>('circle')
    .data(nodes, (node) => node.id)
    .join('circle')
    .attr('fill', '#000')
    .attr('stroke', 'none');

  const syncLinkMask = () => {
    linkMaskNodes
      .attr('cx', (node) => node.x)
      .attr('cy', (node) => node.y)
      .attr('r', (node) => getNodeRadius(node) + 6);
  };

  const getLinkEndpoints = (link: GraphLink) => {
    const source = nodeById.get(link.source);
    const target = nodeById.get(link.target);

    if (!source || !target) {
      return {
        x1: internetPosition.x,
        y1: internetPosition.y,
        x2: internetPosition.x,
        y2: internetPosition.y,
      };
    }

    return {
      x1: source.x,
      y1: source.y,
      x2: target.x,
      y2: target.y,
    };
  };

  const getLinkGeometry = (link: GraphLink): LinkGeometry | null => {
    const source = nodeById.get(link.source);
    const target = nodeById.get(link.target);

    if (!source || !target) return null;

    const points = getLinkEndpoints(link);
    const interfaceNode = source.kind === 'interface' ? source : target;
    const angle = Math.atan2(interfaceNode.y - hubY(), interfaceNode.x - hubX());
    const radialX = Math.cos(angle);
    const radialY = Math.sin(angle);
    const tangentX = -radialY;
    const tangentY = radialX;
    const side = link.id.endsWith('-tx') ? 1 : -1;
    const arcSpread = Math.min(92, Math.max(30, linkWidth(link.speed) * 5.6));
    const outward = Math.min(52, Math.max(16, linkWidth(link.speed) * 2.7));

    return {
      x1: points.x1,
      y1: points.y1,
      c1x: points.x1 + radialX * outward + tangentX * arcSpread * side,
      c1y: points.y1 + radialY * outward + tangentY * arcSpread * side,
      c2x: points.x2 + radialX * outward + tangentX * arcSpread * side,
      c2y: points.y2 + radialY * outward + tangentY * arcSpread * side,
      x2: points.x2,
      y2: points.y2,
    };
  };

  const buildLinkPath = (geometry: LinkGeometry) => `M ${geometry.x1} ${geometry.y1} C ${geometry.c1x} ${geometry.c1y} ${geometry.c2x} ${geometry.c2y} ${geometry.x2} ${geometry.y2}`;

  const buildArrowPath = (geometry: LinkGeometry, speed: number) => {
    const midPoint = getCubicPoint(geometry, 0.5);
    const tangent = getCubicTangent(geometry, 0.5);
    const angle = Math.atan2(tangent.y, tangent.x) * 180 / Math.PI;
    const size = Math.min(28, Math.max(8, linkWidth(speed) * 2.6));
    return {
      path: `M ${-size} ${-size * 0.42} L ${size * 1.15} 0 L ${-size} ${size * 0.42} Z`,
      transform: `translate(${midPoint.x},${midPoint.y}) rotate(${angle})`,
    };
  };

  const linkSelection = svg
    .append('g')
    .attr('fill', 'none')
    .selectAll<SVGPathElement, GraphLink>('path')
    .data(links, (link) => link.id)
    .join('path')
    .attr('class', 'traffic-link')
    .attr('fill', 'none')
    .attr('stroke-linecap', 'round')
    .attr('stroke-linejoin', 'round')
    .attr('stroke-width', (link) => linkWidth(link.speed))
    .attr('stroke', (link) => trafficColor(link.speed))
    .attr('mask', `url(#${maskId})`);

  const arrowSelection = svg
    .append('g')
    .selectAll<SVGPathElement, GraphLink>('path')
    .data(links, (link) => `${link.id}-arrow`)
    .join('path')
    .attr('class', 'traffic-link-arrow')
    .attr('fill', (link) => trafficColor(link.speed))
    .attr('stroke', 'none')
    .attr('pointer-events', 'none')
    .attr('mask', `url(#${maskId})`);

  const updateLinkPaths = () => {
    linkSelection.attr('d', (link) => {
      const geometry = getLinkGeometry(link);
      return geometry ? buildLinkPath(geometry) : '';
    });

    arrowSelection
      .attr('d', (link) => {
        const geometry = getLinkGeometry(link);
        return geometry ? buildArrowPath(geometry, link.speed).path : '';
      })
      .attr('transform', (link) => {
        const geometry = getLinkGeometry(link);
        return geometry ? buildArrowPath(geometry, link.speed).transform : null;
      });
  };

  syncLinkMask();
  updateLinkPaths();

  linkSelection
    .append('title')
    .text((link) => `${link.id.endsWith('-tx') ? 'TX' : 'RX'} ${formatBytes(link.speed)}/s`);

  let currentDragMoved = false;
  let dragOrigins: Map<string, { x: number; y: number }> | null = null;

  const nodeSelection = svg
    .append('g')
    .selectAll<SVGGElement, GraphNode>('g')
    .data(nodes, (node) => node.id)
    .join('g')
    .attr('class', 'traffic-node')
    .attr('transform', (node) => `translate(${node.x},${node.y})`)
    .style('cursor', 'grab')
    .on('click', (event, node) => {
      if (node.kind !== 'interface') return;
      if ((clickBlockUntil.get(node.id) || 0) > Date.now()) {
        event.stopPropagation();
        return;
      }
      clickBlockUntil.delete(node.id);
      event.stopPropagation();
      emit('select-interface', node.id);
    });

  nodeSelection
    .append('circle')
    .attr('r', (node) => (node.kind === 'internet' ? internetRadius : nodeRadius(node.totalSpeed)))
    .attr('fill', (node) => (node.kind === 'internet' ? '#1677ff' : nodeColor(node.totalSpeed)))
    .attr('fill-opacity', (node) => {
      if (node.kind === 'internet') return 0.9;
      return node.totalSpeed > 0 ? 0.85 : 0.55;
    });

  const dragBehavior = d3.drag<SVGGElement, GraphNode>()
    .filter((_event, node) => node.kind === 'interface' || node.kind === 'internet')
    .on('start', function (_event, _node) {
      dragDepth += 1;
      currentDragMoved = false;
      dragOrigins = new Map(nodes.map((item) => [item.id, { x: item.x, y: item.y }]));
      d3.select(this).raise().style('cursor', 'grabbing');
    })
    .on('drag', function (event, node) {
      currentDragMoved = true;
      const radius = getNodeRadius(node);
      const position = clampPosition(event.x, event.y, radius);
      if (node.kind === 'internet' && dragOrigins) {
        const origin = dragOrigins.get('Internet') ?? { x: node.x, y: node.y };
        const deltaX = position.x - origin.x;
        const deltaY = position.y - origin.y;

        nodes.forEach((item) => {
          const start = dragOrigins?.get(item.id) ?? { x: item.x, y: item.y };
          const next = clampPosition(start.x + deltaX, start.y + deltaY, getNodeRadius(item));
          item.x = next.x;
          item.y = next.y;
          nodePositionCache.set(item.id, next);
        });

        orbitSelection
          .attr('cx', position.x)
          .attr('cy', position.y);
        nodeSelection.attr('transform', (item) => `translate(${item.x},${item.y})`);
      } else {
        node.x = position.x;
        node.y = position.y;
        nodePositionCache.set(node.id, position);
        d3.select(this).attr('transform', `translate(${position.x},${position.y})`);
      }
      syncLinkMask();
      updateLinkPaths();
    })
    .on('end', function (_event, node) {
      d3.select(this).style('cursor', 'grab');
      if (currentDragMoved) {
        clickBlockUntil.set(node.id, Date.now() + 250);
      }
      dragOrigins = null;
      currentDragMoved = false;
      dragDepth = Math.max(0, dragDepth - 1);
      if (dragDepth === 0 && renderQueued) {
        renderQueued = false;
        renderGraph();
      }
    });

  nodeSelection.call(dragBehavior as any);

  const textSelection = nodeSelection
    .append('text')
    .attr('text-anchor', 'middle')
    .attr('dy', (node) => (node.kind === 'internet' ? -10 : -8));

  textSelection
    .append('tspan')
    .attr('x', 0)
    .attr('font-size', (node) => (node.kind === 'internet' ? 16 : 12))
    .attr('font-weight', 700)
    .text((node) => node.id);

  textSelection
    .append('tspan')
    .attr('x', 0)
    .attr('dy', (node) => (node.kind === 'internet' ? 18 : 17))
    .attr('font-size', 11)
    .attr('font-weight', 500)
    .text((node) => `↓ ${formatBytes(node.readSpeed)}/s`);

  textSelection
    .append('tspan')
    .attr('x', 0)
    .attr('dy', 17)
    .attr('font-size', 11)
    .attr('font-weight', 500)
    .text((node) => `↑ ${formatBytes(node.writeSpeed)}/s`);

  nodeSelection
    .append('title')
    .text((node) => {
      const lines = [node.id];
      if (node.kind === 'interface') {
        lines.push('Click to view history');
      }
      lines.push(`RX ${formatBytes(node.readSpeed)}/s`);
      lines.push(`TX ${formatBytes(node.writeSpeed)}/s`);
      lines.push(`TOTAL ${formatBytes(node.totalSpeed)}/s`);
      return lines.join('\n');
    });
};

const requestRender = () => {
  if (dragDepth > 0) {
    renderQueued = true;
    return;
  }
  renderQueued = false;
  renderGraph();
};

watch(
  () => props.interfaces,
  () => {
    requestRender();
  },
);

onMounted(() => {
  resizeObserver = new ResizeObserver(() => {
    requestRender();
  });
  if (containerRef.value) {
    resizeObserver.observe(containerRef.value);
  }
  requestRender();
});

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  resizeObserver = null;
});
</script>

<template>
  <div ref="containerRef" class="traffic-graph">
    <svg ref="svgRef" class="traffic-graph__svg" />
  </div>
</template>

<style scoped>
.traffic-graph {
  width: 100%;
  min-height: 420px;
  border-radius: 12px;
  overflow: hidden;
  background: radial-gradient(circle at top, #ffffff 0%, #f8fbff 45%, #eef4ff 100%);
  border: 1px solid #e5eefb;
}

.traffic-graph__svg {
  width: 100%;
  height: 420px;
  display: block;
}

:deep(.traffic-link) {
  stroke-dasharray: 10 8;
  animation: traffic-dash 1.6s linear infinite;
  opacity: 0.9;
  pointer-events: none;
}

:deep(.traffic-link-arrow) {
  pointer-events: none;
}

:deep(.traffic-node) {
  touch-action: none;
  user-select: none;
}

:deep(.traffic-node circle) {
  stroke: rgba(255, 255, 255, 0.96);
  stroke-width: 2.5px;
  filter: drop-shadow(0 10px 18px rgba(15, 23, 42, 0.16));
}

:deep(.traffic-node text) {
  fill: #0f172a;
  paint-order: stroke;
  stroke: rgba(255, 255, 255, 0.95);
  stroke-width: 4px;
  stroke-linejoin: round;
}

@keyframes traffic-dash {
  to {
    stroke-dashoffset: -36;
  }
}
</style>
