<script setup lang="ts">
import { FilterOutlined, ClearOutlined } from "@ant-design/icons-vue";

const props = defineProps<{
  selectedTags: string[];
  selectedTypes: number[];
  timeFilter: string;
  pidFilter: string;
  commandFilter: string;
  pathFilter: string;
  isDeduplicated: boolean;
  hideUnknown: boolean;
  netDirFilter: string;
  syscallCatFilter: string;
  tagOptions: { label: string; value: string }[];
  eventTypeOptions: { label: string; value: number }[];
  networkDirStats: { outgoing: number; incoming: number; listening: number; unknown: number };
  syscallCatStats: Record<string, number>;
  syscallCatLabels: Record<string, string>;
  syscallCatColors: Record<string, string>;
  builtinFilterRules: Array<{ id: string; label: string }>;
  builtinFilterState: Record<string, boolean>;
  builtinFilterSummary: string;
  setBuiltinFiltersEnabled: (enabled: boolean) => void;
  getFilterPopupContainer: (triggerNode: HTMLElement) => HTMLElement;
}>();

const emit = defineEmits<{
  (e: "update:selectedTags", value: string[]): void;
  (e: "update:selectedTypes", value: number[]): void;
  (e: "update:timeFilter", value: string): void;
  (e: "update:pidFilter", value: string): void;
  (e: "update:commandFilter", value: string): void;
  (e: "update:pathFilter", value: string): void;
  (e: "update:isDeduplicated", value: boolean): void;
  (e: "update:hideUnknown", value: boolean): void;
  (e: "update:netDirFilter", value: string): void;
  (e: "update:syscallCatFilter", value: string): void;
  (e: "update:builtinFilterState", id: string, enabled: boolean): void;
  (e: "clearAll"): void;
}">();

const clearAll = () => emit("clearAll");
</script>

<template>
  <div class="filter-panel">
    <div class="filter-panel-header">
      <span class="filter-panel-title">
        <FilterOutlined /> 条件过滤
      </span>
      <a-button size="small" danger @click="clearAll">
        <template #icon><ClearOutlined /></template>
        清除所有过滤条件
      </a-button>
    </div>

    <div class="filter-panel-body">
      <!-- Row 1: Text/Select filters -->
      <div class="filter-row">
        <div class="filter-item">
          <label class="filter-label">Tags</label>
          <a-select
            :value="selectedTags"
            mode="multiple"
            placeholder="All Tags"
            size="small"
            allow-clear
            show-search
            :options="tagOptions"
            option-filter-prop="label"
            :get-popup-container="getFilterPopupContainer"
            style="width: 100%"
            @change="emit('update:selectedTags', $event)"
          />
        </div>

        <div class="filter-item">
          <label class="filter-label">Event Type</label>
          <a-select
            :value="selectedTypes"
            mode="multiple"
            placeholder="All Types"
            size="small"
            allow-clear
            show-search
            :options="eventTypeOptions"
            option-filter-prop="label"
            :get-popup-container="getFilterPopupContainer"
            style="width: 100%"
            @change="emit('update:selectedTypes', $event)"
          />
        </div>

        <div class="filter-item">
          <label class="filter-label">Time</label>
          <a-input
            :value="timeFilter"
            placeholder="Search time..."
            size="small"
            allow-clear
            @update:value="emit('update:timeFilter', $event)"
          />
        </div>
      </div>

      <!-- Row 2: Text filters -->
      <div class="filter-row">
        <div class="filter-item">
          <label class="filter-label">PID</label>
          <a-input
            :value="pidFilter"
            placeholder="PID contains..."
            size="small"
            allow-clear
            @update:value="emit('update:pidFilter', $event)"
          />
        </div>

        <div class="filter-item">
          <label class="filter-label">Command</label>
          <a-input
            :value="commandFilter"
            placeholder="Command contains..."
            size="small"
            allow-clear
            @update:value="emit('update:commandFilter', $event)"
          />
        </div>

        <div class="filter-item">
          <label class="filter-label">Path</label>
          <a-input
            :value="pathFilter"
            placeholder="Path contains..."
            size="small"
            allow-clear
            @update:value="emit('update:pathFilter', $event)"
          />
        </div>
      </div>

      <!-- Row 3: Toggle filters -->
      <div class="filter-row">
        <div class="filter-item filter-item--inline">
          <a-checkbox
            :checked="hideUnknown"
            @update:checked="emit('update:hideUnknown', $event)"
          >
            <span style="font-size: 13px">Hide Unknown</span>
          </a-checkbox>
        </div>

        <div class="filter-item filter-item--inline">
          <a-checkbox
            :checked="isDeduplicated"
            @update:checked="emit('update:isDeduplicated', $event)"
          >
            <span style="font-size: 13px">Clean Duplicates</span>
          </a-checkbox>
        </div>
      </div>

      <!-- Row 4: Network direction filter -->
      <div class="filter-row">
        <div class="filter-item">
          <label class="filter-label">Network Direction</label>
          <div class="filter-tag-group">
            <a-tag
              v-for="d in ['outgoing', 'incoming', 'listening', 'unknown']"
              :key="d"
              :color="netDirFilter === d ? 'blue' : 'default'"
              style="cursor: pointer"
              @click="emit('update:netDirFilter', netDirFilter === d ? 'all' : d)"
            >
              {{ d === "unknown" ? "Unknown" : d.charAt(0).toUpperCase() + d.slice(1) }}
              <span style="margin-left: 2px; font-weight: 600">{{
                (networkDirStats as any)[d]
              }}</span>
            </a-tag>
            <a-tag
              v-if="netDirFilter !== 'all'"
              color="red"
              style="cursor: pointer"
              @click="emit('update:netDirFilter', 'all')"
            >✕ Clear</a-tag>
          </div>
        </div>
      </div>

      <!-- Row 5: Syscall category filter -->
      <div class="filter-row">
        <div class="filter-item">
          <label class="filter-label">Syscall Category</label>
          <div class="filter-tag-group">
            <a-tag
              v-for="cat in Object.keys(syscallCatLabels)"
              :key="cat"
              :color="syscallCatFilter === cat ? syscallCatColors[cat] : 'default'"
              style="cursor: pointer"
              @click="emit('update:syscallCatFilter', syscallCatFilter === cat ? 'all' : cat)"
            >
              {{ syscallCatLabels[cat] }}
              <span style="margin-left: 2px; font-weight: 600">{{
                syscallCatStats[cat] || 0
              }}</span>
            </a-tag>
            <a-tag
              v-if="syscallCatFilter !== 'all'"
              color="red"
              style="cursor: pointer"
              @click="emit('update:syscallCatFilter', 'all')"
            >✕ Clear</a-tag>
          </div>
        </div>
      </div>

      <!-- Row 6: Built-in filters -->
      <div class="filter-row">
        <div class="filter-item filter-item--wide">
          <label class="filter-label">Built-in Filters</label>
          <div class="filter-builtin-row">
            <div class="filter-builtin-summary">
              {{ builtinFilterSummary }}
            </div>
            <div class="filter-builtin-tags">
              <a-tag
                v-for="rule in builtinFilterRules"
                :key="rule.id"
                :color="builtinFilterState[rule.id] !== false ? 'blue' : 'default'"
                style="cursor: pointer"
                @click="emit('update:builtinFilterState', rule.id, builtinFilterState[rule.id] === false)"
              >
                {{ rule.label }}
              </a-tag>
            </div>
            <div class="filter-builtin-actions">
              <a-button size="small" @click="setBuiltinFiltersEnabled(true)">
                Enable All
              </a-button>
              <a-button size="small" @click="setBuiltinFiltersEnabled(false)">
                Disable All
              </a-button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.filter-panel {
  border: 1px solid #d9e4d1;
  border-radius: 6px;
  background: #f8fcf6;
  margin-bottom: 12px;
  overflow: hidden;
}

.filter-panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  background: linear-gradient(180deg, #f7fbf4 0%, #edf4e8 100%);
  border-bottom: 1px solid #d9e4d1;
}

.filter-panel-title {
  font-weight: 700;
  font-size: 14px;
  color: #1f3a1f;
  display: flex;
  align-items: center;
  gap: 6px;
}

.filter-panel-body {
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.filter-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: flex-start;
}

.filter-item {
  flex: 1 1 200px;
  min-width: 180px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.filter-item--inline {
  flex: 0 0 auto;
  min-width: auto;
  flex-direction: row;
  align-items: center;
  padding-top: 18px;
}

.filter-item--wide {
  flex: 1 1 100%;
  min-width: 0;
}

.filter-label {
  font-size: 12px;
  font-weight: 600;
  color: #4b5563;
  letter-spacing: 0.3px;
  text-transform: uppercase;
}

.filter-tag-group {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.filter-builtin-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.filter-builtin-summary {
  font-size: 12px;
  color: #6b7280;
  line-height: 1.4;
  flex: 1 1 100%;
}

.filter-builtin-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.filter-builtin-actions {
  display: flex;
  gap: 6px;
  margin-left: auto;
}
</style>
