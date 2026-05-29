<script setup lang="ts">
import { computed, onMounted, onUnmounted, shallowRef } from 'vue';
import { CompressOutlined, LeftOutlined, MinusOutlined, PlusOutlined, RightOutlined } from '@ant-design/icons-vue';

import {
  filterProcessedEvents,
  formatDuration,
  formatFullTime,
  type ProcessedAgentSightEvent,
} from '../../utils/agentsight';
import AgentSightEventDetails from './AgentSightEventDetails.vue';

const props = defineProps<{
  events: ProcessedAgentSightEvent[];
}>();

const selectedSource = shallowRef<string | undefined>();
const selectedComm = shallowRef<string | undefined>();
const selectedPid = shallowRef<string | undefined>();
const zoomLevel = shallowRef(1);
const scrollOffset = shallowRef(0);
const selectedEvent = shallowRef<ProcessedAgentSightEvent | null>(null);
const detailsOpen = shallowRef(false);

const sourceOptions = computed(() => Array.from(new Set(props.events.map(event => event.source))).sort().map(value => ({ label: value, value })));
const commOptions = computed(() => Array.from(new Set(props.events.map(event => event.comm).filter(Boolean))).sort().map(value => ({ label: value, value })));
const pidOptions = computed(() => Array.from(new Set(props.events.map(event => event.pid).filter(Boolean))).sort((a, b) => a - b).map(value => ({ label: String(value), value: String(value) })));

const filteredEvents = computed(() => filterProcessedEvents(props.events, {
  source: selectedSource.value,
  comm: selectedComm.value,
  pid: selectedPid.value,
}).slice().sort((a, b) => a.timestamp - b.timestamp));

const groupedEvents = computed(() => {
  const groups = new Map<string, ProcessedAgentSightEvent[]>();
  filteredEvents.value.forEach(event => {
    const list = groups.get(event.source) || [];
    list.push(event);
    groups.set(event.source, list);
  });
  return Array.from(groups.entries()).map(([source, events]) => ({ source, events, color: events[0]?.sourceColor || '#64748b' }));
});

const fullTimeRange = computed(() => {
  if (filteredEvents.value.length === 0) return { start: 0, end: 0, span: 1 };
  const timestamps = filteredEvents.value.map(event => event.timestamp);
  const start = Math.min(...timestamps);
  const end = Math.max(...timestamps);
  return { start, end, span: Math.max(end - start, 1) };
});

const visibleTimeRange = computed(() => {
  if (zoomLevel.value <= 1) return fullTimeRange.value;
  const zoomedSpan = fullTimeRange.value.span / zoomLevel.value;
  const maxOffset = Math.max(fullTimeRange.value.span - zoomedSpan, 0);
  const clampedOffset = Math.max(0, Math.min(scrollOffset.value, maxOffset));
  return {
    start: fullTimeRange.value.start + clampedOffset,
    end: fullTimeRange.value.start + clampedOffset + zoomedSpan,
    span: Math.max(zoomedSpan, 1),
  };
});

const durationLabel = computed(() => formatDuration(fullTimeRange.value.span));
const scrollProgress = computed(() => {
  const zoomedSpan = fullTimeRange.value.span / zoomLevel.value;
  const maxOffset = Math.max(fullTimeRange.value.span - zoomedSpan, 0);
  return maxOffset > 0 ? scrollOffset.value / maxOffset : 0;
});
const thumbWidth = computed(() => Math.max((visibleTimeRange.value.span / fullTimeRange.value.span) * 100, 5));
const thumbLeft = computed(() => scrollProgress.value * (100 - thumbWidth.value));

const eventPosition = (timestamp: number) => ((timestamp - visibleTimeRange.value.start) / visibleTimeRange.value.span) * 100;
const minimapPosition = (timestamp: number) => ((timestamp - fullTimeRange.value.start) / fullTimeRange.value.span) * 100;

const zoomTo = (next: number) => {
  const current = zoomLevel.value;
  const target = Math.max(1, Math.min(next, 12));
  const currentCenter = scrollOffset.value + (fullTimeRange.value.span / current) / 2;
  const nextSpan = fullTimeRange.value.span / target;
  const maxOffset = Math.max(fullTimeRange.value.span - nextSpan, 0);
  zoomLevel.value = target;
  scrollOffset.value = target === 1 ? 0 : Math.max(0, Math.min(maxOffset, currentCenter - nextSpan / 2));
};

const zoomIn = () => zoomTo(zoomLevel.value * 1.5);
const zoomOut = () => zoomTo(zoomLevel.value / 1.5);
const resetZoom = () => {
  zoomLevel.value = 1;
  scrollOffset.value = 0;
};
const scrollBy = (ratio: number) => {
  if (zoomLevel.value <= 1) return;
  const visibleSpan = fullTimeRange.value.span / zoomLevel.value;
  const maxOffset = Math.max(fullTimeRange.value.span - visibleSpan, 0);
  scrollOffset.value = Math.max(0, Math.min(maxOffset, scrollOffset.value + visibleSpan * ratio));
};

const handleWheel = (event: WheelEvent) => {
  if (event.ctrlKey || event.metaKey) {
    event.preventDefault();
    if (event.deltaY < 0) zoomIn();
    else zoomOut();
    return;
  }
  if (zoomLevel.value > 1) {
    event.preventDefault();
    scrollBy(event.deltaY > 0 ? 0.1 : -0.1);
  }
};

const handleKeydown = (event: KeyboardEvent) => {
  if (event.ctrlKey || event.metaKey) {
    if (event.key === '=' || event.key === '+') {
      event.preventDefault();
      zoomIn();
    } else if (event.key === '-') {
      event.preventDefault();
      zoomOut();
    } else if (event.key === '0') {
      event.preventDefault();
      resetZoom();
    }
    return;
  }
  if (zoomLevel.value > 1 && event.key === 'ArrowLeft') {
    event.preventDefault();
    scrollBy(-0.1);
  } else if (zoomLevel.value > 1 && event.key === 'ArrowRight') {
    event.preventDefault();
    scrollBy(0.1);
  }
};

const jumpToMinimap = (event: MouseEvent) => {
  if (zoomLevel.value <= 1) return;
  const target = event.currentTarget as HTMLElement;
  const rect = target.getBoundingClientRect();
  const clickPosition = (event.clientX - rect.left) / rect.width;
  const maxOffset = Math.max(fullTimeRange.value.span - visibleTimeRange.value.span, 0);
  scrollOffset.value = Math.max(0, Math.min(maxOffset, clickPosition * maxOffset));
};

const openDetails = (event: ProcessedAgentSightEvent) => {
  selectedEvent.value = event;
  detailsOpen.value = true;
};

onMounted(() => window.addEventListener('keydown', handleKeydown));
onUnmounted(() => window.removeEventListener('keydown', handleKeydown));
</script>

<template>
  <div class="timeline-view">
    <div class="timeline-toolbar">
      <div class="timeline-title">
        <strong>Timeline</strong>
        <span>{{ filteredEvents.length }} events · {{ groupedEvents.length }} lanes · {{ durationLabel }}</span>
      </div>
      <a-space wrap>
        <a-button size="small" @click="zoomOut"><template #icon><MinusOutlined /></template></a-button>
        <a-tag color="blue">{{ Math.round(zoomLevel * 100) }}%</a-tag>
        <a-button size="small" @click="zoomIn"><template #icon><PlusOutlined /></template></a-button>
        <a-button size="small" @click="resetZoom"><template #icon><CompressOutlined /></template>Reset</a-button>
        <a-button v-if="zoomLevel > 1" size="small" @click="scrollBy(-0.1)"><template #icon><LeftOutlined /></template></a-button>
        <a-button v-if="zoomLevel > 1" size="small" @click="scrollBy(0.1)"><template #icon><RightOutlined /></template></a-button>
      </a-space>
    </div>

    <div class="timeline-filters">
      <a-select v-model:value="selectedSource" allow-clear placeholder="Source" :options="sourceOptions" class="filter-select" />
      <a-select v-model:value="selectedComm" allow-clear show-search placeholder="Process" :options="commOptions" class="filter-select" />
      <a-select v-model:value="selectedPid" allow-clear show-search placeholder="PID" :options="pidOptions" class="filter-select small" />
      <span class="help">Ctrl/⌘ + wheel zooms, wheel pans when zoomed, ←/→ scroll.</span>
    </div>

    <a-empty v-if="filteredEvents.length === 0" description="No timeline events" />
    <div v-else class="timeline-canvas" @wheel="handleWheel">
      <div class="axis">
        <span>{{ formatFullTime(visibleTimeRange.start) }}</span>
        <div class="ticks">
          <span v-for="i in 5" :key="i" :style="{ left: `${((i - 1) / 4) * 100}%` }">
            {{ new Date(visibleTimeRange.start + visibleTimeRange.span * ((i - 1) / 4)).toLocaleTimeString(undefined, { hour12: false }) }}
          </span>
        </div>
        <span>{{ formatFullTime(visibleTimeRange.end) }}</span>
      </div>

      <div v-if="zoomLevel > 1" class="scrollbar" @click="jumpToMinimap">
        <div class="scrollbar-thumb" :style="{ left: `${thumbLeft}%`, width: `${thumbWidth}%` }" />
      </div>

      <div class="minimap" @click="jumpToMinimap">
        <span
          v-for="event in filteredEvents"
          :key="`mini-${event.id}`"
          class="minimap-dot"
          :style="{ left: `${minimapPosition(event.timestamp)}%`, background: event.sourceColor }"
        />
        <div class="minimap-window" :style="{ left: `${(scrollOffset / fullTimeRange.span) * 100}%`, width: `${(visibleTimeRange.span / fullTimeRange.span) * 100}%` }" />
      </div>

      <div class="lanes">
        <div v-for="group in groupedEvents" :key="group.source" class="lane">
          <div class="lane-label">
            <span class="lane-dot" :style="{ background: group.color }" />
            <strong>{{ group.source }}</strong>
            <a-tag>{{ group.events.length }}</a-tag>
          </div>
          <div class="lane-track">
            <button
              v-for="event in group.events"
              v-show="eventPosition(event.timestamp) >= 0 && eventPosition(event.timestamp) <= 100"
              :key="event.id"
              class="event-marker"
              :style="{ left: `${eventPosition(event.timestamp)}%`, background: group.color }"
              :title="`${event.formattedTime} · ${event.comm}#${event.pid} · ${event.title}`"
              @click="openDetails(event)"
            />
          </div>
        </div>
      </div>
    </div>

    <AgentSightEventDetails :open="detailsOpen" :event="selectedEvent" @close="detailsOpen = false" />
  </div>
</template>

<style scoped>
.timeline-view {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.timeline-toolbar,
.timeline-filters {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
}

.timeline-title {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.timeline-title span,
.help {
  color: #64748b;
  font-size: 12px;
}

.filter-select {
  width: 180px;
}

.filter-select.small {
  width: 120px;
}

.timeline-canvas {
  padding: 14px;
  border: 1px solid #e5e7eb;
  border-radius: 14px;
  background: linear-gradient(180deg, #ffffff 0%, #f8fafc 100%);
  overflow: hidden;
}

.axis {
  display: grid;
  grid-template-columns: 220px 1fr 220px;
  gap: 10px;
  align-items: start;
  min-height: 48px;
  color: #475569;
  font-size: 12px;
}

.axis > span:last-child {
  text-align: right;
}

.ticks {
  position: relative;
  height: 36px;
  border-bottom: 1px solid #cbd5e1;
}

.ticks span {
  position: absolute;
  top: 18px;
  transform: translateX(-50%);
  white-space: nowrap;
  color: #94a3b8;
}

.ticks span::before {
  content: '';
  position: absolute;
  left: 50%;
  bottom: 18px;
  width: 1px;
  height: 12px;
  background: #cbd5e1;
}

.scrollbar,
.minimap {
  position: relative;
  border-radius: 999px;
  cursor: pointer;
}

.scrollbar {
  height: 12px;
  margin-bottom: 10px;
  background: #e2e8f0;
}

.scrollbar-thumb {
  position: absolute;
  top: 0;
  bottom: 0;
  border-radius: 999px;
  background: #1677ff;
}

.minimap {
  height: 22px;
  margin-bottom: 18px;
  background: #e2e8f0;
  overflow: hidden;
}

.minimap-dot {
  position: absolute;
  top: 6px;
  width: 3px;
  height: 10px;
  border-radius: 2px;
  opacity: 0.75;
}

.minimap-window {
  position: absolute;
  top: 0;
  bottom: 0;
  border: 1px solid #1677ff;
  background: rgba(22, 119, 255, 0.16);
}

.lanes {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.lane {
  display: grid;
  grid-template-columns: 180px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
}

.lane-label {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.lane-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  flex: none;
}

.lane-track {
  position: relative;
  height: 32px;
  border-radius: 8px;
  background: rgba(226, 232, 240, 0.6);
}

.event-marker {
  position: absolute;
  top: 5px;
  width: 8px;
  height: 22px;
  padding: 0;
  border: 0;
  border-radius: 4px;
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.24);
  cursor: pointer;
  transform: translateX(-50%);
}

.event-marker:hover {
  outline: 2px solid #0f172a;
}
</style>
