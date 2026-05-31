<script setup lang="ts">
import { FilterOutlined } from "@ant-design/icons-vue";

defineProps<{
  isConnected: boolean;
  eventsLength: number;
  isPaused: boolean;
  streamDirection: "top" | "bottom";
  showAllRows: boolean;
  hideUnknown: boolean;
  isDeduplicated: boolean;
  maxEvents: number;
  maxEventsOptions: string[];
  builtinFilterSummary: string;
  builtinFilterRules: Array<{ id: string; label: string }>;
  builtinFilterState: Record<string, boolean>;
  setBuiltinFiltersEnabled: (enabled: boolean) => void;
  exportEvents: () => void;
  exportEventsCsv: () => void;
  clearEvents: () => void;
}>();

const emit = defineEmits<{
  (e: "update:isPaused", value: boolean): void;
  (e: "update:streamDirection", value: "top" | "bottom"): void;
  (e: "update:showAllRows", value: boolean): void;
  (e: "update:hideUnknown", value: boolean): void;
  (e: "update:isDeduplicated", value: boolean): void;
  (e: "update:maxEvents", value: number): void;
}>();
</script>

<template>
  <div class="dashboard-toolbar">
    <div
      style="
        display: flex;
        justify-content: space-between;
        align-items: center;
        gap: 12px;
        flex-wrap: wrap;
        width: 100%;
      "
    >
      <div style="display: flex; align-items: center; gap: 16px">
        <a-badge
          :status="isConnected ? 'success' : 'error'"
          :text="isConnected ? 'Connected' : 'Disconnected'"
        />
        <span style="font-weight: 500">Total Events: {{ eventsLength }}</span>
        <a-divider type="vertical" />
        <a-button
          @click="emit('update:isPaused', !isPaused)"
          :type="isPaused ? 'primary' : 'default'"
          size="small"
          danger
        >
          {{ isPaused ? "Resume Stream" : "Pause Stream" }}
        </a-button>
        <a-button type="primary" danger size="small" @click="clearEvents"
          >Clear Events</a-button
        >
        <a-select
          :value="streamDirection"
          size="small"
          style="width: 150px"
          @update:value="emit('update:streamDirection', $event)"
        >
          <a-select-option value="top">Newest First</a-select-option>
          <a-select-option value="bottom">Log Flow ↓</a-select-option>
        </a-select>
        <a-checkbox
          :checked="showAllRows"
          @update:checked="emit('update:showAllRows', $event)"
        >
          <span style="font-size: 12px">No Page Limit</span>
        </a-checkbox>
        <a-checkbox
          :checked="hideUnknown"
          size="small"
          @update:checked="emit('update:hideUnknown', $event)"
        >
          <span style="font-size: 12px">Hide Unknown</span>
        </a-checkbox>
        <a-checkbox
          :checked="isDeduplicated"
          size="small"
          @update:checked="emit('update:isDeduplicated', $event)"
        >
          <span style="font-size: 12px">Clean Duplicates</span>
        </a-checkbox>
        <a-popover trigger="click" placement="bottomLeft" :arrow="false">
          <template #content>
            <div class="builtin-filter-popover">
              <div class="builtin-filter-popover-title">Built-in Filters</div>
              <div class="builtin-filter-popover-summary">
                {{ builtinFilterSummary }}
              </div>
              <a-space direction="vertical" :size="4" style="width: 100%">
                <a-checkbox
                  v-for="rule in builtinFilterRules"
                  :key="rule.id"
                  v-model:checked="builtinFilterState[rule.id]"
                >
                  {{ rule.label }}
                </a-checkbox>
              </a-space>
              <div class="builtin-filter-popover-actions">
                <a-button size="small" @click="setBuiltinFiltersEnabled(true)"
                  >Enable All</a-button
                >
                <a-button size="small" @click="setBuiltinFiltersEnabled(false)"
                  >Disable All</a-button
                >
              </div>
            </div>
          </template>
          <a-tag
            color="blue"
            style="cursor: pointer"
            :title="builtinFilterSummary"
          >
            <FilterOutlined /> Built-in Filters
          </a-tag>
        </a-popover>
      </div>
      <div style="display: flex; gap: 8px; align-items: center">
        <span style="font-size: 12px; color: #888">Max:</span>
        <a-select
          :value="maxEvents"
          size="small"
          style="width: 80px"
          @update:value="emit('update:maxEvents', $event)"
        >
          <a-select-option
            v-for="opt in maxEventsOptions"
            :key="opt"
            :value="Number(opt)"
            >{{ opt }}</a-select-option
          >
        </a-select>
        <a-dropdown>
          <template #overlay>
            <a-menu>
              <a-menu-item key="json" @click="exportEvents"
                >JSON Format</a-menu-item
              >
              <a-menu-item key="csv" @click="exportEventsCsv"
                >CSV Format</a-menu-item
              >
            </a-menu>
          </template>
          <a-button size="small">Export Data</a-button>
        </a-dropdown>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dashboard-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  border: 1px solid #d9e4d1;
  border-radius: 6px;
  padding: 12px 14px;
  background: #f8fcf6;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.75);
  justify-content: space-between;
  margin-bottom: 10px;
}

.builtin-filter-popover {
  min-width: 220px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.builtin-filter-popover-title {
  font-size: 13px;
  font-weight: 600;
  color: #1f2937;
}

.builtin-filter-popover-summary {
  font-size: 12px;
  color: #6b7280;
  line-height: 1.4;
}

.builtin-filter-popover-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  flex-wrap: wrap;
}
</style>
