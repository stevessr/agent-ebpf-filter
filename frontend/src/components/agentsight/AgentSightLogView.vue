<script setup lang="ts">
import { computed, shallowRef } from "vue";
import { SearchOutlined } from "@ant-design/icons-vue";

import {
  filterProcessedEvents,
  type ProcessedAgentSightEvent,
} from "../../utils/agentsight";
import AgentSightEventDetails from "./AgentSightEventDetails.vue";

const props = defineProps<{
  events: ProcessedAgentSightEvent[];
}>();

const searchTerm = shallowRef("");
const selectedSource = shallowRef<string | undefined>();
const selectedComm = shallowRef<string | undefined>();
const selectedPid = shallowRef<string | undefined>();
const selectedEvent = shallowRef<ProcessedAgentSightEvent | null>(null);
const detailsOpen = shallowRef(false);

const sourceOptions = computed(() =>
  Array.from(new Set(props.events.map((event) => event.source)))
    .sort()
    .map((value) => ({
      label: `${value} (${props.events.filter((event) => event.source === value).length})`,
      value,
    })),
);
const commOptions = computed(() =>
  Array.from(new Set(props.events.map((event) => event.comm).filter(Boolean)))
    .sort()
    .map((value) => ({
      label: `${value} (${props.events.filter((event) => event.comm === value).length})`,
      value,
    })),
);
const pidOptions = computed(() =>
  Array.from(new Set(props.events.map((event) => event.pid).filter(Boolean)))
    .sort((a, b) => a - b)
    .map((value) => ({
      label: `PID ${value} (${props.events.filter((event) => event.pid === value).length})`,
      value: String(value),
    })),
);

const filteredEvents = computed(() =>
  filterProcessedEvents(props.events, {
    source: selectedSource.value,
    comm: selectedComm.value,
    pid: selectedPid.value,
    searchTerm: searchTerm.value,
  }),
);

const openDetails = (event: ProcessedAgentSightEvent) => {
  selectedEvent.value = event;
  detailsOpen.value = true;
};
</script>

<template>
  <div class="agentsight-log-view">
    <div class="log-filters">
      <a-input
        v-model:value="searchTerm"
        allow-clear
        placeholder="Search events, payloads, IDs"
        class="search-input"
      >
        <template #prefix><SearchOutlined /></template>
      </a-input>
      <a-select
        v-model:value="selectedSource"
        allow-clear
        placeholder="All sources"
        :options="sourceOptions"
        class="filter-select"
      />
      <a-select
        v-model:value="selectedComm"
        allow-clear
        show-search
        placeholder="All processes"
        :options="commOptions"
        class="filter-select"
      />
      <a-select
        v-model:value="selectedPid"
        allow-clear
        show-search
        placeholder="All PIDs"
        :options="pidOptions"
        class="filter-select small"
      />
      <a-tag color="blue"
        >{{ filteredEvents.length }} / {{ events.length }}</a-tag
      >
    </div>

    <a-empty
      v-if="filteredEvents.length === 0"
      description="No AgentSight events match the current filters"
    />
    <a-list
      v-else
      class="log-list"
      :data-source="filteredEvents"
      item-layout="vertical"
    >
      <template #renderItem="{ item }">
        <a-list-item class="log-row" @click="openDetails(item)">
          <div class="log-row-main">
            <div class="log-row-meta">
              <a-typography-text code>{{
                item.formattedTime
              }}</a-typography-text>
              <a-tag :color="item.sourceColorClass">{{ item.source }}</a-tag>
              <a-tag color="geekblue">{{ item.eventType }}</a-tag>
              <span class="process">{{ item.comm }}#{{ item.pid || "—" }}</span>
              <a-tag v-if="item.redactionState" color="green">{{
                item.redactionState
              }}</a-tag>
            </div>
            <div class="summary">{{ item.title }}</div>
            <div class="event-id">{{ item.id }}</div>
          </div>
        </a-list-item>
      </template>
    </a-list>

    <AgentSightEventDetails
      :open="detailsOpen"
      :event="selectedEvent"
      @close="detailsOpen = false"
    />
  </div>
</template>

<style scoped>
.agentsight-log-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.log-filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.search-input {
  min-width: 260px;
  flex: 1 1 320px;
}

.filter-select {
  width: 210px;
}

.filter-select.small {
  width: 150px;
}

.log-list {
  max-height: 620px;
  overflow: auto;
  border: 1px solid #f0f0f0;
  border-radius: 10px;
}

.log-row {
  cursor: pointer;
  transition:
    background 0.16s ease,
    transform 0.16s ease;
}

.log-row:hover {
  background: #f8fafc;
  transform: translateX(2px);
}

.log-row-main {
  min-width: 0;
  width: 100%;
}

.log-row-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.process {
  color: #475569;
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.summary {
  color: #111827;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.event-id {
  margin-top: 4px;
  color: #64748b;
  font-size: 11px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>
