<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue';
import { CompressOutlined, EyeOutlined } from '@ant-design/icons-vue';

import { formatFullTime, type ProcessedAgentSightEvent } from '../../utils/agentsight';
import {
  agentSightFlameBreadcrumbs,
  agentSightFlameMetricValue,
  buildAgentSightFlameTree,
  findAgentSightFlameNode,
  formatAgentSightFlameMetric,
  layoutAgentSightFlamegraph,
  type AgentSightFlameDimensionPreset,
  type AgentSightFlameMetric,
  type AgentSightFlameNode,
  type AgentSightFlameRect,
} from '../../utils/agentsightFlamegraph';
import AgentSightEventDetails from './AgentSightEventDetails.vue';

const props = defineProps<{
  events: ProcessedAgentSightEvent[];
}>();

const metric = shallowRef<AgentSightFlameMetric>('count');
const dimensionPreset = shallowRef<AgentSightFlameDimensionPreset>('execution');
const focusedNodeId = shallowRef('root');
const selectedNodeId = shallowRef('root');
const selectedEvent = shallowRef<ProcessedAgentSightEvent | null>(null);
const detailsOpen = shallowRef(false);

const metricOptions: Array<{ label: string; value: AgentSightFlameMetric }> = [
  { label: 'Count', value: 'count' },
  { label: 'Bytes', value: 'bytes' },
  { label: 'Duration', value: 'duration' },
  { label: 'Risk', value: 'risk' },
];

const dimensionOptions: Array<{ label: string; value: AgentSightFlameDimensionPreset }> = [
  { label: 'Execution', value: 'execution' },
  { label: 'Process', value: 'process' },
  { label: 'Conversation', value: 'conversation' },
];

const tree = computed(() => buildAgentSightFlameTree(props.events, dimensionPreset.value));
const focusedNode = computed(() => findAgentSightFlameNode(tree.value, focusedNodeId.value) || tree.value);
const selectedNode = computed(() => findAgentSightFlameNode(tree.value, selectedNodeId.value) || focusedNode.value);
const breadcrumbs = computed(() => agentSightFlameBreadcrumbs(tree.value, focusedNode.value.id));
const rects = computed(() => layoutAgentSightFlamegraph(focusedNode.value, metric.value));
const visibleRects = computed(() => rects.value.filter(rect => focusedNode.value.id !== 'root' || rect.node.level !== 'root'));
const hasMetricValue = computed(() => agentSightFlameMetricValue(tree.value, metric.value) > 0);
const hasFocusedMetricValue = computed(() => agentSightFlameMetricValue(focusedNode.value, metric.value) > 0);
const displayY = (rect: AgentSightFlameRect) => focusedNode.value.id === 'root' ? rect.y - 30 : rect.y;
const svgHeight = computed(() => Math.max(96, visibleRects.value.reduce((max, rect) => Math.max(max, displayY(rect) + rect.height), 0) + 8));
const eventCount = computed(() => props.events.length.toLocaleString());
const currentMetricLabel = computed(() => metricOptions.find(option => option.value === metric.value)?.label || metric.value);
const rootMetricValue = computed(() => agentSightFlameMetricValue(tree.value, metric.value));
const focusedMetricValue = computed(() => agentSightFlameMetricValue(focusedNode.value, metric.value));
const selectedMetricValue = computed(() => agentSightFlameMetricValue(selectedNode.value, metric.value));
const selectedRootPercent = computed(() => rootMetricValue.value > 0 ? selectedMetricValue.value / rootMetricValue.value : 0);
const selectedFocusPercent = computed(() => focusedMetricValue.value > 0 ? selectedMetricValue.value / focusedMetricValue.value : 0);
const sourceLegend = computed(() => {
  const sources = new Map<string, { source: string; color: string; count: number }>();
  visibleRects.value.forEach(rect => {
    if (rect.node.level === 'other' || !rect.node.dominantSource) return;
    const current = sources.get(rect.node.dominantSource) || { source: rect.node.dominantSource, color: rect.node.dominantColor, count: 0 };
    current.count += rect.node.eventCount;
    sources.set(rect.node.dominantSource, current);
  });
  return Array.from(sources.values()).sort((a, b) => b.count - a.count).slice(0, 8);
});

const resetFocus = () => {
  focusedNodeId.value = 'root';
  selectedNodeId.value = 'root';
};

const focusNode = (node: AgentSightFlameNode) => {
  if (node.level === 'other') return;
  focusedNodeId.value = node.id;
  selectedNodeId.value = node.id;
};

const selectNode = (node: AgentSightFlameNode) => {
  selectedNodeId.value = node.id;
  if (node.children.length > 0 && node.level !== 'other') {
    focusNode(node);
    return;
  }
  if (node.representativeEvent) {
    selectedEvent.value = node.representativeEvent;
    detailsOpen.value = true;
  }
};

const openEvent = (event?: ProcessedAgentSightEvent | null) => {
  if (!event) return;
  selectedEvent.value = event;
  detailsOpen.value = true;
};

const closeDetails = () => {
  detailsOpen.value = false;
};

const formatMetric = (value: number, nextMetric = metric.value) => formatAgentSightFlameMetric(value, nextMetric);
const percentLabel = (value: number) => `${(value * 100).toFixed(value >= 0.1 ? 1 : 2)}%`;
const labelVisible = (rect: AgentSightFlameRect) => rect.width >= 56;
const valueVisible = (rect: AgentSightFlameRect) => rect.width >= 110;

const rectFill = (node: AgentSightFlameNode) => node.level === 'other' ? '#94a3b8' : node.dominantColor || '#64748b';
const rectOpacity = (node: AgentSightFlameNode) => String(Math.max(0.58, 0.94 - Math.max(0, node.depth - focusedNode.value.depth) * 0.055));
const tooltipText = (rect: AgentSightFlameRect) => [
  `${rect.node.label}`,
  `Level: ${rect.node.level}`,
  `${currentMetricLabel.value}: ${formatMetric(rect.value)}`,
  `Root: ${percentLabel(rect.percentOfRoot)}`,
  `Parent: ${percentLabel(rect.percentOfParent)}`,
  `Events: ${rect.node.eventCount.toLocaleString()}`,
  `Dominant source: ${rect.node.dominantSource || 'unknown'}`,
  `Latest: ${formatFullTime(rect.node.latestTimestamp)}`,
  `Representative: ${rect.node.representativeEvent?.title || '—'}`,
].join('\n');

watch([() => props.events, dimensionPreset], () => resetFocus());
watch(metric, () => {
  if (!hasMetricValue.value) selectedNodeId.value = focusedNode.value.id;
});
</script>

<template>
  <div class="flamegraph-view">
    <div class="flamegraph-toolbar">
      <div class="flamegraph-heading">
        <strong>Flamegraph</strong>
        <span>{{ eventCount }} events · hierarchical event aggregation, not CPU stack sampling</span>
      </div>
      <a-space wrap>
        <a-segmented v-model:value="metric" :options="metricOptions" />
        <a-segmented v-model:value="dimensionPreset" :options="dimensionOptions" />
        <a-button size="small" :disabled="focusedNode.id === 'root'" @click="resetFocus">
          <template #icon><CompressOutlined /></template>
          Reset Zoom
        </a-button>
      </a-space>
    </div>

    <a-alert
      type="info"
      show-icon
      message="事件层级火焰图"
      description="宽度表示所选指标在层级路径中的聚合占比，不表示真实时间轴；CPU/perf 函数级火焰图需要单独的 stack 采集链路。"
      class="flamegraph-note"
    />

    <div class="flamegraph-overview">
      <a-tag color="blue">Focused: {{ focusedNode.label }}</a-tag>
      <a-tag>{{ currentMetricLabel }} {{ formatMetric(focusedMetricValue) }}</a-tag>
      <a-tag>{{ focusedNode.eventCount.toLocaleString() }} events in focus</a-tag>
      <span v-if="sourceLegend.length > 0" class="source-legend">
        <span v-for="source in sourceLegend" :key="source.source" class="source-chip">
          <span class="source-dot" :style="{ background: source.color }" />
          {{ source.source }}
        </span>
      </span>
    </div>

    <div v-if="breadcrumbs.length > 1" class="flamegraph-breadcrumbs">
      <a-breadcrumb>
        <a-breadcrumb-item v-for="node in breadcrumbs" :key="node.id">
          <a-button type="link" size="small" @click="focusNode(node)">{{ node.label }}</a-button>
        </a-breadcrumb-item>
      </a-breadcrumb>
    </div>

    <a-empty v-if="events.length === 0" description="No AgentSight events match current filters" />
    <a-empty v-else-if="!hasMetricValue" :description="`No non-zero values for the selected metric: ${currentMetricLabel}`" />
    <a-empty v-else-if="!hasFocusedMetricValue" :description="`The focused subtree has no non-zero ${currentMetricLabel} values`">
      <template #extra>
        <a-button size="small" @click="resetFocus">Reset Zoom</a-button>
      </template>
    </a-empty>
    <template v-else>
      <div class="flamegraph-canvas">
        <svg class="flamegraph-svg" :viewBox="`0 0 1200 ${svgHeight}`" preserveAspectRatio="none" role="img" aria-label="AgentSight flamegraph">
          <g v-for="rect in visibleRects" :key="rect.node.id">
            <rect
              class="flamegraph-rect"
              :class="{ selected: selectedNode.id === rect.node.id }"
              :x="rect.x"
              :y="displayY(rect)"
              :width="rect.width"
              :height="rect.height"
              rx="4"
              :fill="rectFill(rect.node)"
              :opacity="rectOpacity(rect.node)"
              @click="selectNode(rect.node)"
            >
              <title>{{ tooltipText(rect) }}</title>
            </rect>
            <text
              v-if="labelVisible(rect)"
              class="flamegraph-label"
              :x="rect.x + 7"
              :y="displayY(rect) + 16"
              @click="selectNode(rect.node)"
            >
              {{ rect.node.label }}
            </text>
            <text
              v-if="valueVisible(rect)"
              class="flamegraph-value"
              :x="rect.x + rect.width - 7"
              :y="displayY(rect) + 16"
              text-anchor="end"
              @click="selectNode(rect.node)"
            >
              {{ formatMetric(rect.value) }}
            </text>
          </g>
        </svg>
      </div>

      <a-card size="small" class="flamegraph-summary" :title="selectedNode.label">
        <template #extra>
          <a-space wrap>
            <a-tag color="blue">{{ selectedNode.level }}</a-tag>
            <a-tag>{{ selectedNode.eventCount.toLocaleString() }} events</a-tag>
            <a-button size="small" :disabled="selectedNode.children.length === 0 || selectedNode.level === 'other'" @click="focusNode(selectedNode)">
              Zoom here
            </a-button>
            <a-button size="small" :disabled="!selectedNode.representativeEvent" @click="openEvent(selectedNode.representativeEvent)">
              <template #icon><EyeOutlined /></template>
              Open sample event
            </a-button>
          </a-space>
        </template>

        <a-row :gutter="[12, 12]">
          <a-col :xs="12" :md="6"><a-statistic title="Count" :value="selectedNode.metrics.count" /></a-col>
          <a-col :xs="12" :md="6"><a-statistic title="Bytes" :value="formatMetric(selectedNode.metrics.bytes, 'bytes')" /></a-col>
          <a-col :xs="12" :md="6"><a-statistic title="Duration" :value="formatMetric(selectedNode.metrics.duration, 'duration')" /></a-col>
          <a-col :xs="12" :md="6"><a-statistic title="Risk" :value="formatMetric(selectedNode.metrics.risk, 'risk')" /></a-col>
        </a-row>

        <div class="node-meta">
          <span>Selected {{ currentMetricLabel }}: <strong>{{ formatMetric(selectedMetricValue) }}</strong></span>
          <span>Root share: <strong>{{ percentLabel(selectedRootPercent) }}</strong></span>
          <span>Focused share: <strong>{{ percentLabel(selectedFocusPercent) }}</strong></span>
          <span>Latest: <strong>{{ formatFullTime(selectedNode.latestTimestamp) }}</strong></span>
          <span>Dominant source: <strong>{{ selectedNode.dominantSource || 'unknown' }}</strong></span>
        </div>

        <a-list v-if="selectedNode.sampleEvents.length > 0" size="small" :data-source="selectedNode.sampleEvents" class="sample-list">
          <template #header>Sample events</template>
          <template #renderItem="{ item }">
            <a-list-item @click="openEvent(item)">
              <a-list-item-meta :description="`${item.formattedTime} · ${item.comm}#${item.pid} · ${item.eventType}`">
                <template #title>
                  <a-space wrap>
                    <a-tag :color="item.sourceColorClass">{{ item.source }}</a-tag>
                    <span>{{ item.title }}</span>
                  </a-space>
                </template>
              </a-list-item-meta>
            </a-list-item>
          </template>
        </a-list>
      </a-card>
    </template>

    <AgentSightEventDetails :open="detailsOpen" :event="selectedEvent" @close="closeDetails" />
  </div>
</template>

<style scoped>
.flamegraph-view {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.flamegraph-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.flamegraph-heading {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.flamegraph-heading span {
  color: #64748b;
  font-size: 12px;
}

.flamegraph-note {
  margin-bottom: 0;
}

.flamegraph-overview {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.source-legend {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  color: #64748b;
  font-size: 12px;
}

.source-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.source-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
}

.flamegraph-breadcrumbs {
  padding: 6px 10px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #f8fafc;
}

.flamegraph-breadcrumbs :deep(.ant-btn-link) {
  height: auto;
  padding: 0;
}

.flamegraph-canvas {
  overflow-x: auto;
  padding: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  background: linear-gradient(180deg, #f8fafc 0%, #ffffff 100%);
}

.flamegraph-svg {
  min-width: 960px;
  width: 100%;
  cursor: pointer;
  overflow: visible;
}

.flamegraph-rect {
  stroke: rgba(15, 23, 42, 0.22);
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
  transition: opacity 0.15s ease, stroke 0.15s ease;
}

.flamegraph-rect:hover,
.flamegraph-rect.selected {
  opacity: 1;
  stroke: #0f172a;
  stroke-width: 1.5;
}

.flamegraph-label,
.flamegraph-value {
  fill: #0f172a;
  font-size: 12px;
  font-weight: 650;
  pointer-events: auto;
  user-select: none;
  paint-order: stroke;
  stroke: rgba(255, 255, 255, 0.76);
  stroke-width: 3px;
  stroke-linejoin: round;
}

.flamegraph-value {
  font-weight: 700;
}

.flamegraph-summary {
  overflow: hidden;
}

.flamegraph-summary :deep(.ant-statistic-title) {
  color: #64748b;
  font-size: 12px;
}

.flamegraph-summary :deep(.ant-statistic-content) {
  color: #0f172a;
  font-size: 18px;
  font-weight: 700;
}

.node-meta {
  display: flex;
  gap: 10px 16px;
  flex-wrap: wrap;
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  background: #f8fafc;
  color: #475569;
  font-size: 12px;
}

.node-meta strong {
  color: #0f172a;
}

.sample-list :deep(.ant-list-item) {
  cursor: pointer;
  border-radius: 8px;
  padding-inline: 8px;
}

.sample-list :deep(.ant-list-item:hover) {
  background: #f8fafc;
}
</style>
