<script setup lang="ts">
import { computed, ref, shallowRef } from 'vue';
import { FilterOutlined } from '@ant-design/icons-vue';

import {
  buildProcessTree,
  createDefaultProcessFilters,
  extractProcessFilterOptions,
  filterProcessTree,
  getTotalEventCount,
  type AgentSightEvent,
  type AgentSightProcessNode as AgentSightProcessNodeType,
  type AgentSightProcessFilters,
} from '../../utils/agentsight';
import AgentSightProcessNode from './AgentSightProcessNode.vue';

const props = defineProps<{
  events: AgentSightEvent[];
}>();

const expandedProcesses = ref<Set<number>>(new Set());
const expandedEvents = ref<Set<string>>(new Set());
const filters = ref<AgentSightProcessFilters>(createDefaultProcessFilters());
const filtersOpen = shallowRef(false);

const processTree = computed(() => buildProcessTree(props.events));
const filterOptions = computed(() => extractProcessFilterOptions(props.events));
const filteredTree = computed(() => filterProcessTree(processTree.value, filters.value));
const totalEvents = computed(() => getTotalEventCount(processTree.value));
const filteredEvents = computed(() => getTotalEventCount(filteredTree.value));
const hasActiveFilters = computed(() => filters.value.eventTypes.length > 0 || filters.value.models.length > 0 || filters.value.sources.length > 0 || filters.value.commands.length > 0 || filters.value.searchText.length > 0 || Boolean(filters.value.timeRange.start || filters.value.timeRange.end));

const eventTypeOptions = computed(() => filterOptions.value.eventTypes.map(value => ({ label: value, value })));
const modelOptions = computed(() => filterOptions.value.models.map(value => ({ label: value, value })));
const sourceOptions = computed(() => filterOptions.value.sources.map(value => ({ label: value, value })));
const commandOptions = computed(() => filterOptions.value.commands.map(value => ({ label: value, value })));

const toggleProcess = (pid: number) => {
  const next = new Set(expandedProcesses.value);
  if (next.has(pid)) next.delete(pid);
  else next.add(pid);
  expandedProcesses.value = next;
};

const toggleEvent = (id: string) => {
  const next = new Set(expandedEvents.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  expandedEvents.value = next;
};

const collectProcessPids = (nodes: AgentSightProcessNodeType[]): number[] => nodes.flatMap(node => [node.pid, ...collectProcessPids(node.children)]);

const expandAllProcesses = () => {
  expandedProcesses.value = new Set(collectProcessPids(filteredTree.value));
};

const collapseAllProcesses = () => {
  expandedProcesses.value = new Set();
  expandedEvents.value = new Set();
};

const expandCurrentEvents = () => {
  expandedEvents.value = new Set(filteredTree.value.flatMap(function collect(node: AgentSightProcessNodeType): string[] {
    return [...node.events.map(event => event.id), ...node.children.flatMap(collect)];
  }));
};

const updateFilter = <K extends keyof AgentSightProcessFilters>(key: K, value: AgentSightProcessFilters[K]) => {
  filters.value = { ...filters.value, [key]: value };
};

const setPreset = (eventTypes: string[]) => {
  updateFilter('eventTypes', eventTypes);
  filtersOpen.value = true;
};

const clearFilters = () => {
  filters.value = createDefaultProcessFilters();
};

const normalizePickerText = (text: string | string[]) => (Array.isArray(text) ? text[0] : text);
const normalizeStringArray = (value: unknown) => Array.isArray(value) ? value.map(String) : [];

const onSearchInput = (event: Event) => {
  updateFilter('searchText', (event.target as HTMLInputElement).value);
};

const setStartTime = (value: string) => {
  updateFilter('timeRange', { ...filters.value.timeRange, start: value ? new Date(value).getTime() : undefined });
};

const setEndTime = (value: string) => {
  updateFilter('timeRange', { ...filters.value.timeRange, end: value ? new Date(value).getTime() : undefined });
};

const onStartPickerChange = (_date: unknown, text: string | string[]) => setStartTime(normalizePickerText(text));
const onEndPickerChange = (_date: unknown, text: string | string[]) => setEndTime(normalizePickerText(text));
const onEventTypesChange = (value: unknown) => updateFilter('eventTypes', normalizeStringArray(value));
const onModelsChange = (value: unknown) => updateFilter('models', normalizeStringArray(value));
const onSourcesChange = (value: unknown) => updateFilter('sources', normalizeStringArray(value));
const onCommandsChange = (value: unknown) => updateFilter('commands', normalizeStringArray(value));
</script>

<template>
  <div class="process-tree-view">
    <div class="tree-header">
      <div>
        <h3>Process Tree</h3>
        <p>Hierarchical AgentSight process relationships with prompt, response, TLS, file, stdio, policy, and process blocks.</p>
      </div>
      <a-space wrap>
        <a-tag color="blue">{{ filteredEvents }} / {{ totalEvents }} events</a-tag>
        <a-button size="small" @click="filtersOpen = !filtersOpen">
          <template #icon><FilterOutlined /></template>
          Filters
          <a-badge v-if="hasActiveFilters" status="processing" />
        </a-button>
        <a-button size="small" @click="expandAllProcesses">Expand all</a-button>
        <a-button size="small" @click="expandCurrentEvents">Expand events</a-button>
        <a-button size="small" @click="collapseAllProcesses">Collapse</a-button>
        <a-button v-if="hasActiveFilters" size="small" @click="clearFilters">Clear</a-button>
      </a-space>
    </div>

    <div class="preset-row">
      <a-button size="small" @click="setPreset(['prompt', 'response'])">AI only</a-button>
      <a-button size="small" @click="setPreset(['file'])">Files only</a-button>
      <a-button size="small" @click="setPreset(['process'])">Processes only</a-button>
      <a-button size="small" @click="setPreset(['stdio'])">Stdio/MCP only</a-button>
      <a-button size="small" @click="setPreset(['ssl'])">TLS/HTTP only</a-button>
    </div>

    <a-card v-if="filtersOpen" size="small" class="filter-card">
      <a-row :gutter="[12, 12]">
        <a-col :xs="24" :md="12">
          <a-input :value="filters.searchText" placeholder="Search in titles, payloads, commands, models" allow-clear @change="onSearchInput" />
        </a-col>
        <a-col :xs="24" :md="12">
          <a-space wrap>
            <a-date-picker show-time placeholder="Start" @change="onStartPickerChange" />
            <a-date-picker show-time placeholder="End" @change="onEndPickerChange" />
          </a-space>
        </a-col>
        <a-col :xs="24" :md="12">
          <label>Event types</label>
          <a-select :value="filters.eventTypes" mode="multiple" allow-clear :options="eventTypeOptions" style="width: 100%" @change="onEventTypesChange" />
        </a-col>
        <a-col :xs="24" :md="12">
          <label>Models</label>
          <a-select :value="filters.models" mode="multiple" allow-clear :options="modelOptions" style="width: 100%" @change="onModelsChange" />
        </a-col>
        <a-col :xs="24" :md="12">
          <label>Sources</label>
          <a-select :value="filters.sources" mode="multiple" allow-clear :options="sourceOptions" style="width: 100%" @change="onSourcesChange" />
        </a-col>
        <a-col :xs="24" :md="12">
          <label>Commands</label>
          <a-select :value="filters.commands" mode="multiple" allow-clear show-search :options="commandOptions" style="width: 100%" @change="onCommandsChange" />
        </a-col>
      </a-row>
    </a-card>

    <a-empty v-if="filteredTree.length === 0" :description="totalEvents === 0 ? 'No process events loaded' : 'No processes match the filters'" />
    <div v-else class="tree-body">
      <AgentSightProcessNode
        v-for="process in filteredTree"
        :key="process.pid"
        :process="process"
        :depth="0"
        :expanded-processes="expandedProcesses"
        :expanded-events="expandedEvents"
        @toggle-process="toggleProcess"
        @toggle-event="toggleEvent"
      />
    </div>
  </div>
</template>

<style scoped>
.process-tree-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.tree-header {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
}

.tree-header h3 {
  margin: 0;
  color: #0f172a;
}

.tree-header p {
  margin: 4px 0 0;
  color: #64748b;
}

.preset-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.filter-card label {
  display: block;
  margin-bottom: 4px;
  color: #475569;
  font-size: 12px;
  font-weight: 600;
}

.tree-body {
  max-height: 720px;
  overflow: auto;
  padding: 8px;
  border: 1px solid #f0f0f0;
  border-radius: 12px;
}
</style>
