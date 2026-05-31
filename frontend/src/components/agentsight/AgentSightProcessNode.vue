<script setup lang="ts">
import { computed } from "vue";
import {
  CaretDownOutlined,
  CaretRightOutlined,
  CodeOutlined,
  FileTextOutlined,
  LockOutlined,
  RobotOutlined,
} from "@ant-design/icons-vue";

import {
  buildParsedEventPreview,
  formatFullTime,
  parsedTypeColor,
  type AgentSightProcessNode,
  type AgentSightTimelineItem,
  type ParsedAgentSightEvent,
} from "../../utils/agentsight";

const props = defineProps<{
  process: AgentSightProcessNode;
  depth: number;
  expandedProcesses: Set<number>;
  expandedEvents: Set<string>;
}>();

const emit = defineEmits<{
  toggleProcess: [pid: number];
  toggleEvent: [id: string];
}>();

const isExpanded = computed(() =>
  props.expandedProcesses.has(props.process.pid),
);
const eventCounts = computed(() =>
  props.process.events.reduce<Record<string, number>>((counts, event) => {
    counts[event.type] = (counts[event.type] || 0) + 1;
    return counts;
  }, {}),
);

const iconFor = (event: ParsedAgentSightEvent) => {
  if (
    event.type === "prompt" ||
    event.type === "response" ||
    event.type === "agent"
  )
    return RobotOutlined;
  if (event.type === "file") return FileTextOutlined;
  if (event.type === "stdio") return CodeOutlined;
  return LockOutlined;
};

const renderableTimeline = computed(() =>
  props.process.timeline.filter((item) => item.event || item.process),
);
const eventTagText = (event: ParsedAgentSightEvent) =>
  event.type === "prompt" && event.promptDiff?.hasChanges
    ? "prompt changed"
    : event.type;
const itemKey = (item: AgentSightTimelineItem, index: number) =>
  item.event?.id || `process-${item.process?.pid ?? index}`;
</script>

<template>
  <div class="process-node">
    <button
      class="process-header"
      :style="{ marginLeft: `${depth * 24}px` }"
      @click="emit('toggleProcess', process.pid)"
    >
      <CaretDownOutlined v-if="isExpanded" />
      <CaretRightOutlined v-else />
      <span class="pid">PID {{ process.pid }}</span>
      <strong>[{{ process.comm }}]</strong>
      <span v-if="process.ppid" class="ppid">← {{ process.ppid }}</span>
      <span class="badges">
        <a-tag
          v-for="(count, type) in eventCounts"
          :key="type"
          :color="parsedTypeColor(type as any)"
          >{{ type }} {{ count }}</a-tag
        >
      </span>
    </button>

    <div
      v-if="isExpanded"
      class="timeline"
      :style="{ marginLeft: `${depth * 24 + 34}px` }"
    >
      <template
        v-for="(item, index) in renderableTimeline"
        :key="itemKey(item, index)"
      >
        <AgentSightProcessNode
          v-if="item.process"
          :process="item.process"
          :depth="depth + 1"
          :expanded-processes="expandedProcesses"
          :expanded-events="expandedEvents"
          @toggle-process="(pid) => emit('toggleProcess', pid)"
          @toggle-event="(id) => emit('toggleEvent', id)"
        />
        <div
          v-else-if="item.event"
          class="event-block"
          :class="`event-${item.event.type}`"
          @click="emit('toggleEvent', item.event.id)"
        >
          <div class="event-head">
            <component :is="iconFor(item.event)" class="event-icon" />
            <a-tag :color="parsedTypeColor(item.event.type)">{{
              eventTagText(item.event)
            }}</a-tag>
            <a-tag v-if="item.event.metadata.model">{{
              item.event.metadata.model
            }}</a-tag>
            <span class="event-title">{{ item.event.title }}</span>
            <span class="event-time">{{
              formatFullTime(item.event.timestamp)
            }}</span>
            <CaretDownOutlined v-if="expandedEvents.has(item.event.id)" />
            <CaretRightOutlined v-else-if="item.event.content.length > 300" />
          </div>
          <div v-if="!expandedEvents.has(item.event.id)" class="event-preview">
            {{ buildParsedEventPreview(item.event) }}
          </div>
          <pre v-else class="event-content">{{
            item.event.promptDiff?.diff
              ? `=== CHANGES FROM PREVIOUS PROMPT ===\n${item.event.promptDiff.diff}\n\n=== FULL CONTENT ===\n${item.event.content}`
              : item.event.content
          }}</pre>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.process-node {
  width: 100%;
}

.process-header {
  display: flex;
  width: calc(100% - 4px);
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border: 0;
  border-left: 3px solid #818cf8;
  border-radius: 0 10px 10px 0;
  background: transparent;
  text-align: left;
  cursor: pointer;
  transition: background 0.16s ease;
}

.process-header:hover {
  background: #f8fafc;
}

.pid {
  padding: 2px 7px;
  border-radius: 6px;
  background: #eef2ff;
  color: #3730a3;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
}

.ppid {
  color: #94a3b8;
  font-size: 12px;
}

.badges {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 4px;
}

.timeline {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 4px;
  margin-bottom: 8px;
}

.event-block {
  padding: 9px 10px;
  border-left: 4px solid #f59e0b;
  border-radius: 10px;
  background: linear-gradient(90deg, #fff7ed, #fffbeb);
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.08);
  cursor: pointer;
}

.event-prompt {
  border-left-color: #3b82f6;
  background: linear-gradient(90deg, #eff6ff, #f5f3ff);
}

.event-response {
  border-left-color: #10b981;
  background: linear-gradient(90deg, #ecfdf5, #f0fdfa);
}

.event-file {
  border-left-color: #06b6d4;
  background: linear-gradient(90deg, #ecfeff, #eff6ff);
}

.event-process {
  border-left-color: #8b5cf6;
  background: linear-gradient(90deg, #f5f3ff, #faf5ff);
}

.event-stdio {
  border-left-color: #6366f1;
  background: linear-gradient(90deg, #eef2ff, #f0f9ff);
}

.event-policy {
  border-left-color: #ef4444;
  background: linear-gradient(90deg, #fef2f2, #fff7ed);
}

.event-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.event-icon {
  flex: none;
  color: #475569;
}

.event-title {
  flex: 1 1 auto;
  min-width: 120px;
  overflow: hidden;
  color: #0f172a;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.event-time {
  flex: none;
  color: #64748b;
  font-size: 12px;
}

.event-preview {
  margin-top: 6px;
  overflow: hidden;
  color: #475569;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.event-content {
  max-height: 360px;
  overflow: auto;
  margin: 8px 0 0;
  padding: 10px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.72);
  color: #1e293b;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
}
</style>
