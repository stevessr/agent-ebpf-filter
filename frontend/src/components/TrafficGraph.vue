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

const props = defineProps<{
  interfaces: TrafficInterface[];
}>();

const emit = defineEmits<{
  (event: 'select-interface', name: string): void;
}>();

const containerRef = ref<HTMLElement | null>(null);
const svgRef = ref<SVGSVGElement | null>(null);

let resizeObserver: ResizeObserver | null = null;

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

const buildMarker = (defs: d3.Selection<SVGDefsElement, unknown, null, undefined>, id: string, color: string) => {
  defs
    .append('marker')
    .attr('id', id)
    .attr('viewBox', '0 0 10 10')
    .attr('refX', 10)
    .attr('refY', 5)
    .attr('markerWidth', 7)
    .attr('markerHeight', 7)
    .attr('orient', 'auto')
    .append('path')
    .attr('d', 'M 0 0 L 10 5 L 0 10 z')
    .attr('fill', color);
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
  const nodeRadius = d3.scaleSqrt().domain([0, maxSpeed]).range([22, 56]);
  const linkWidth = d3.scaleSqrt().domain([0, maxSpeed]).range([1.5, 10]);
  const orbitRadius = Math.min(
    Math.max(Math.min(width, height) * 0.34, 120),
    Math.max(Math.min(width, height) / 2 - 84, 76),
  );
  const internetRadius = 64;

  const nodes: GraphNode[] = [
    {
      id: 'Internet',
      kind: 'internet',
      readSpeed: aggregateIn,
      writeSpeed: aggregateOut,
      totalSpeed: aggregateIn + aggregateOut,
      x: centerX,
      y: centerY,
    },
    ...interfaces.map((item, index) => {
      const angle = Math.PI + (index / interfaces.length) * Math.PI * 2;
      const positionRadius = orbitRadius;
      return {
        id: item.name,
        kind: 'interface' as const,
        readSpeed: item.readSpeed,
        writeSpeed: item.writeSpeed,
        totalSpeed: item.totalSpeed,
        x: centerX + Math.cos(angle) * positionRadius,
        y: centerY + Math.sin(angle) * positionRadius,
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

  const defs = svg.append('defs');
  buildMarker(defs, 'traffic-arrow-low', '#52c41a');
  buildMarker(defs, 'traffic-arrow-mid', '#faad14');
  buildMarker(defs, 'traffic-arrow-high', '#ff4d4f');

  svg
    .append('circle')
    .attr('cx', centerX)
    .attr('cy', centerY)
    .attr('r', orbitRadius)
    .attr('fill', 'none')
    .attr('stroke', 'rgba(148, 163, 184, 0.25)')
    .attr('stroke-dasharray', '6 10');

  const getNodeRadius = (node: GraphNode) => (node.kind === 'internet' ? internetRadius : nodeRadius(node.totalSpeed));

  const getLinkEndpoints = (link: GraphLink) => {
    const source = nodeById.get(link.source);
    const target = nodeById.get(link.target);

    if (!source || !target) {
      return {
        x1: centerX,
        y1: centerY,
        x2: centerX,
        y2: centerY,
      };
    }

    const dx = target.x - source.x;
    const dy = target.y - source.y;
    const distance = Math.max(1, Math.hypot(dx, dy));
    const sourceOffset = Math.min(getNodeRadius(source) + 4, distance / 2 - 1);
    const targetOffset = Math.min(getNodeRadius(target) + 8, distance / 2 - 1);

    return {
      x1: source.x + (dx * sourceOffset) / distance,
      y1: source.y + (dy * sourceOffset) / distance,
      x2: target.x - (dx * targetOffset) / distance,
      y2: target.y - (dy * targetOffset) / distance,
    };
  };

  const linkGeometry = new Map(
    links.map((link) => {
      const base = getLinkEndpoints(link);
      const source = nodeById.get(link.source)!;
      const target = nodeById.get(link.target)!;
      const dx = target.x - source.x;
      const dy = target.y - source.y;
      const distance = Math.max(1, Math.hypot(dx, dy));
      const normalX = -dy / distance;
      const normalY = dx / distance;
      const direction = link.id.endsWith('-tx') ? 1 : -1;
      const laneOffset = Math.min(16, Math.max(6, linkWidth(link.speed) * 0.8));
      const offsetX = normalX * direction * laneOffset;
      const offsetY = normalY * direction * laneOffset;
      return [
        link.id,
        {
          x1: base.x1 + offsetX,
          y1: base.y1 + offsetY,
          x2: base.x2 + offsetX,
          y2: base.y2 + offsetY,
        },
      ] as const;
    }),
  );

  const linkSelection = svg
    .append('g')
    .attr('fill', 'none')
    .selectAll<SVGLineElement, GraphLink>('line')
    .data(links, (link) => link.id)
    .join('line')
    .attr('class', 'traffic-link')
    .attr('stroke-linecap', 'round')
    .attr('stroke-width', (link) => linkWidth(link.speed))
    .attr('stroke', (link) => trafficColor(link.speed))
    .attr('marker-end', (link) => {
      const color = trafficColor(link.speed);
      if (color === '#ff4d4f') return 'url(#traffic-arrow-high)';
      if (color === '#faad14') return 'url(#traffic-arrow-mid)';
      return 'url(#traffic-arrow-low)';
    });

  linkSelection
    .attr('x1', (link) => linkGeometry.get(link.id)?.x1 ?? centerX)
    .attr('y1', (link) => linkGeometry.get(link.id)?.y1 ?? centerY)
    .attr('x2', (link) => linkGeometry.get(link.id)?.x2 ?? centerX)
    .attr('y2', (link) => linkGeometry.get(link.id)?.y2 ?? centerY);

  linkSelection
    .append('title')
    .text((link) => `${link.id.endsWith('-tx') ? 'TX' : 'RX'} ${formatBytes(link.speed)}/s`);

  const nodeSelection = svg
    .append('g')
    .selectAll<SVGGElement, GraphNode>('g')
    .data(nodes, (node) => node.id)
    .join('g')
    .attr('class', 'traffic-node')
    .attr('transform', (node) => `translate(${node.x},${node.y})`)
    .style('cursor', (node) => (node.kind === 'interface' ? 'pointer' : 'default'))
    .on('click', (event, node) => {
      if (node.kind !== 'interface') return;
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

watch(
  () => props.interfaces,
  () => {
    renderGraph();
  },
);

onMounted(() => {
  resizeObserver = new ResizeObserver(() => {
    renderGraph();
  });
  if (containerRef.value) {
    resizeObserver.observe(containerRef.value);
  }
  renderGraph();
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
