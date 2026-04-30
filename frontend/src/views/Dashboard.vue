<script setup lang="ts">
import { EyeOutlined, FilterOutlined, FolderOpenOutlined, InfoCircleOutlined } from '@ant-design/icons-vue';

import FilePreviewDrawer from '../components/FilePreviewDrawer.vue';
import { useDashboard } from '../composables/useDashboard';

const {
  events,
  isConnected,
  isPaused,
  showDetails,
  selectedEvent,
  showPreview,
  previewLoading,
  previewData,
  selectedTags,
  selectedTypes,
  timeFilter,
  pidFilter,
  commandFilter,
  pathFilter,
  isDeduplicated,
  hideUnknown,
  activeHeaderFilter,
  tableWrapperRef,
  streamDirection,
  showAllRows,
  builtinFilterRules,
  builtinFilterState,
  builtinFilterSummary,
  setBuiltinFiltersEnabled,
  maxEvents,
  maxEventsOptions,
  activeTab,
  netDirFilter,
  syscallCatFilter,
  categoryTabs,
  networkDirStats,
  syscallCatStats,
  syscallCatLabels,
  syscallCatColors,
  tableColumns,
  tablePagination,
  eventTypeOptions,
  tagOptions,
  displayedEvents,
  openDetails,
  formatDetailValue,
  canInteractWithPath,
  previewRecordPath,
  openInExplorer,
  getTagColor,
  getCategoryColor,
  getRowClassName,
  onTabChange,
  handleTableChange,
  toggleHeaderFilter,
  clearHeaderFilter,
  isHeaderFilterActive,
  hasHeaderFilter,
  isResizableColumn,
  startColumnResize,
  getFilterPopupContainer,
  clearEvents,
  exportEvents,
  exportEventsCSV,
  syscallDisplayName,
} = useDashboard();

void tableWrapperRef;
</script>

<template>
  <div class="dashboard-page">
    <a-tabs
      :activeKey="activeTab"
      size="small"
      @change="onTabChange"
      class="dashboard-tabs"
    >
      <a-tab-pane v-for="tab in categoryTabs" :key="tab.key" :tab="tab.label" />
    </a-tabs>
    <div class="dashboard-toolbar">
      <div style="display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; width: 100%;">
        <div style="display: flex; align-items: center; gap: 16px;">
          <a-badge :status="isConnected ? 'success' : 'error'" :text="isConnected ? 'Connected' : 'Disconnected'" />
          <span style="font-weight: 500;">Total Events: {{ events.length }}</span>
          <a-divider type="vertical" />
          <a-button @click="isPaused = !isPaused" :type="isPaused ? 'primary' : 'default'" size="small" danger>
            {{ isPaused ? 'Resume Stream' : 'Pause Stream' }}
          </a-button>
          <a-button type="primary" danger size="small" @click="clearEvents">Clear Events</a-button>
          <a-select
            v-model:value="streamDirection"
            size="small"
            style="width: 150px;"
          >
            <a-select-option value="top">Newest First</a-select-option>
            <a-select-option value="bottom">Log Flow ↓</a-select-option>
          </a-select>
          <a-checkbox v-model:checked="showAllRows">
            <span style="font-size: 12px;">No Page Limit</span>
          </a-checkbox>
          <a-checkbox v-model:checked="hideUnknown" size="small">
            <span style="font-size: 12px;">Hide Unknown</span>
          </a-checkbox>
          <a-checkbox v-model:checked="isDeduplicated" size="small">
            <span style="font-size: 12px;">Clean Duplicates</span>
          </a-checkbox>
          <a-popover trigger="click" placement="bottomLeft" :arrow="false">
            <template #content>
              <div class="builtin-filter-popover">
                <div class="builtin-filter-popover-title">Built-in Filters</div>
                <div class="builtin-filter-popover-summary">{{ builtinFilterSummary }}</div>
                <a-space direction="vertical" :size="4" style="width: 100%;">
                  <a-checkbox
                    v-for="rule in builtinFilterRules"
                    :key="rule.id"
                    v-model:checked="builtinFilterState[rule.id]"
                  >
                    {{ rule.label }}
                  </a-checkbox>
                </a-space>
                <div class="builtin-filter-popover-actions">
                  <a-button size="small" @click="setBuiltinFiltersEnabled(true)">Enable All</a-button>
                  <a-button size="small" @click="setBuiltinFiltersEnabled(false)">Disable All</a-button>
                </div>
              </div>
            </template>
            <a-tag
              color="blue"
              style="cursor: pointer;"
              :title="builtinFilterSummary"
            >
              Built-in Filters
            </a-tag>
          </a-popover>
        </div>
        <div style="display: flex; gap: 8px; align-items: center;">
          <span style="font-size: 12px; color: #888;">Max:</span>
          <a-select v-model:value="maxEvents" size="small" style="width: 80px">
            <a-select-option v-for="opt in maxEventsOptions" :key="opt" :value="Number(opt)">{{ opt }}</a-select-option>
          </a-select>
          <a-dropdown>
            <template #overlay>
              <a-menu>
                <a-menu-item key="json" @click="exportEvents">JSON Format</a-menu-item>
                <a-menu-item key="csv" @click="exportEventsCSV">CSV Format</a-menu-item>
              </a-menu>
            </template>
            <a-button size="small">Export Data</a-button>
          </a-dropdown>
        </div>
      </div>
    </div>

    <div v-if="activeTab === 'network'" class="net-dir-bar">
      <span style="font-weight: 600; margin-right: 8px; color: #555;">Direction:</span>
      <a-tag
        v-for="d in ['outgoing','incoming','listening','unknown']" :key="d"
        :color="netDirFilter === d ? 'blue' : 'default'"
        style="cursor: pointer;"
        @click="netDirFilter = netDirFilter === d ? 'all' : d"
      >
        {{ d === 'unknown' ? 'Unknown' : d.charAt(0).toUpperCase() + d.slice(1) }}
        <span style="margin-left: 2px; font-weight: 600;">{{ (networkDirStats as any)[d] }}</span>
      </a-tag>
      <a-tag v-if="netDirFilter !== 'all'" color="red" style="cursor: pointer;" @click="netDirFilter = 'all'">✕ Clear</a-tag>
    </div>

    <div v-if="activeTab === 'syscall'" class="net-dir-bar">
      <span style="font-weight: 600; margin-right: 8px; color: #555;">Category:</span>
      <a-tag
        v-for="cat in Object.keys(syscallCatLabels)" :key="cat"
        :color="syscallCatFilter === cat ? syscallCatColors[cat] : 'default'"
        style="cursor: pointer;"
        @click="syscallCatFilter = syscallCatFilter === cat ? 'all' : cat"
      >
        {{ syscallCatLabels[cat] }}
        <span style="margin-left: 2px; font-weight: 600;">{{ syscallCatStats[cat] || 0 }}</span>
      </a-tag>
      <a-tag v-if="syscallCatFilter !== 'all'" color="red" style="cursor: pointer;" @click="syscallCatFilter = 'all'">✕ Clear</a-tag>
    </div>

    <div ref="tableWrapperRef" class="dashboard-table-wrap">
      <a-table
        class="excel-table"
        :dataSource="displayedEvents"
        :columns="tableColumns"
        size="small"
        :pagination="tablePagination"
        :rowClassName="getRowClassName"
        :tableLayout="'fixed'"
        @change="handleTableChange"
      >
      <template #headerCell="{ column }">
        <div class="excel-header-cell">
          <span class="excel-header-title">{{ column.title }}</span>
          <div class="excel-header-actions">
            <a-popover
              v-if="hasHeaderFilter(column.key)"
              trigger="click"
              placement="bottomRight"
              :arrow="false"
              :open="activeHeaderFilter === column.key"
              overlay-class-name="excel-filter-popover"
            >
              <template #content>
                <div
                  class="excel-filter-dropdown"
                  :class="{ 'excel-filter-dropdown--wide': column.key === 'tag' || column.key === 'type' }"
                  @mousedown.stop
                  @click.stop
                >
                  <div class="excel-filter-dropdown-title">
                    {{ column.title }} Filter
                  </div>
                  <template v-if="column.key === 'time'">
                    <a-input
                      v-model:value="timeFilter"
                      placeholder="Search time..."
                      size="small"
                      allow-clear
                    />
                  </template>
                  <template v-else-if="column.key === 'tag'">
                    <a-select
                      v-model:value="selectedTags"
                      mode="multiple"
                      placeholder="All Tags"
                      size="small"
                      allow-clear
                      show-search
                      :options="tagOptions"
                      option-filter-prop="label"
                      :get-popup-container="getFilterPopupContainer"
                      style="width: 100%;"
                    />
                  </template>
                  <template v-else-if="column.key === 'pid'">
                    <a-input
                      v-model:value="pidFilter"
                      placeholder="PID contains..."
                      size="small"
                      allow-clear
                    />
                  </template>
                  <template v-else-if="column.key === 'comm'">
                    <a-input
                      v-model:value="commandFilter"
                      placeholder="Command contains..."
                      size="small"
                      allow-clear
                    />
                  </template>
                  <template v-else-if="column.key === 'type'">
                    <a-select
                      v-model:value="selectedTypes"
                      mode="multiple"
                      placeholder="All Types"
                      size="small"
                      allow-clear
                      show-search
                      :options="eventTypeOptions"
                      option-filter-prop="label"
                      :get-popup-container="getFilterPopupContainer"
                      style="width: 100%;"
                    />
                  </template>
                  <template v-else-if="column.key === 'path'">
                    <a-input
                      v-model:value="pathFilter"
                      placeholder="Path contains..."
                      size="small"
                      allow-clear
                    />
                  </template>

                  <div class="excel-filter-dropdown-actions">
                    <a-button size="small" :disabled="!isHeaderFilterActive(column.key)" @click="clearHeaderFilter(column.key)">
                      Clear
                    </a-button>
                  </div>
                </div>
              </template>
              <a-button
                type="text"
                size="small"
                class="excel-header-filter-trigger"
                :class="{ active: isHeaderFilterActive(column.key) }"
                @click.stop="toggleHeaderFilter(column.key)"
              >
                <template #icon><FilterOutlined /></template>
              </a-button>
            </a-popover>
            <span
              v-if="isResizableColumn(column.key)"
              class="excel-column-resizer"
              title="Drag to resize"
              @mousedown.stop.prevent="startColumnResize(column.key, $event)"
            />
          </div>
        </div>
      </template>
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'type'">
          <div class="excel-type-cell">
            <a-tag :color="getTagColor(record.eventType, record.type)">
              {{ record.type === 'syscall' && syscallDisplayName(record.extraInfo) ? syscallDisplayName(record.extraInfo) : record.type.toUpperCase() }}
            </a-tag>
            <a-tag v-if="(record.occurrenceCount ?? 1) > 1" color="blue" class="excel-occurrence-tag">
              ×{{ record.occurrenceCount }}
            </a-tag>
          </div>
        </template>
        <template v-if="column.key === 'tag'">
          <a-tag :color="getCategoryColor(record.tag)">{{ record.tag }}</a-tag>
        </template>
        <template v-if="column.key === 'path'">
          <div class="excel-path-cell">
            <a-typography-text
              class="excel-path-text"
              :style="{ cursor: canInteractWithPath(record) ? 'pointer' : 'default' }"
              @click="previewRecordPath(record)"
            >
              {{ formatDetailValue(record.path) }}
            </a-typography-text>
            <a-tooltip v-if="canInteractWithPath(record)" title="Preview file">
              <a-button type="link" size="small" @click.stop="previewRecordPath(record)">
                <template #icon><EyeOutlined /></template>
              </a-button>
            </a-tooltip>
            <a-tooltip v-if="canInteractWithPath(record)" title="Open in Explorer">
              <a-button type="link" size="small" @click.stop="openInExplorer(record)">
                <template #icon><FolderOpenOutlined /></template>
              </a-button>
            </a-tooltip>
          </div>
        </template>
        <template v-if="column.key === 'action'">
          <a-button type="link" size="small" @click="openDetails(record)">
            <template #icon><InfoCircleOutlined /></template>
          </a-button>
        </template>
      </template>
      </a-table>
    </div>

    <a-modal v-model:open="showDetails" title="Event Details" :footer="null" width="600px">
      <a-descriptions bordered :column="1" size="small" v-if="selectedEvent">
        <a-descriptions-item label="Time">{{ selectedEvent.time }}</a-descriptions-item>
        <a-descriptions-item label="Event Type">
          <a-tag :color="getTagColor(selectedEvent.eventType, selectedEvent.type)">{{ selectedEvent.type.toUpperCase() }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Tag">
          <a-tag :color="getCategoryColor(selectedEvent.tag)">{{ selectedEvent.tag }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Command"><a-typography-text strong>{{ selectedEvent.comm }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="PID"><code>{{ formatDetailValue(selectedEvent.pid) }}</code></a-descriptions-item>
        <a-descriptions-item label="Parent PID (PPID)"><code>{{ formatDetailValue(selectedEvent.ppid) }}</code></a-descriptions-item>
        <a-descriptions-item label="User ID (UID)"><code>{{ formatDetailValue(selectedEvent.uid) }}</code></a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.netDirection" label="Network Direction">
          <a-tag color="blue">{{ selectedEvent.netDirection }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.netEndpoint" label="Network Endpoint">
          <a-typography-text code style="word-break: break-all;">{{ selectedEvent.netEndpoint }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.netFamily" label="Network Family">
          <a-tag color="purple">{{ selectedEvent.netFamily }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.netBytes !== undefined" label="Network Bytes">
          <a-typography-text code>{{ selectedEvent.netBytes }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.retval !== undefined" label="Return Value">
          <a-typography-text :type="selectedEvent.retval < 0 ? 'danger' : undefined" code>{{ selectedEvent.retval }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item v-if="(selectedEvent.occurrenceCount ?? 1) > 1" label="Occurrences">
          <a-tag color="blue">×{{ selectedEvent.occurrenceCount ?? 1 }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.extraInfo" label="Extra Info">
          <a-typography-text code>{{ selectedEvent.extraInfo }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.extraPath" label="Extra Path">
          <code style="word-break: break-all;">{{ selectedEvent.extraPath }}</code>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.bytes !== undefined" label="Bytes">
          <a-typography-text code>{{ selectedEvent.bytes }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.mode" label="Mode">
          <a-typography-text code>{{ selectedEvent.mode }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.domain" label="Domain">
          <a-tag>{{ selectedEvent.domain }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.sockType" label="Socket Type">
          <a-tag>{{ selectedEvent.sockType }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.protocol !== undefined" label="Protocol">
          <a-typography-text code>{{ selectedEvent.protocol }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.uidArg !== undefined" label="Chown UID">
          <a-typography-text code>{{ selectedEvent.uidArg }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.gidArg !== undefined" label="Chown GID">
          <a-typography-text code>{{ selectedEvent.gidArg }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item label="Resource Path / Info">
          <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
            <code style="word-break: break-all;">{{ formatDetailValue(selectedEvent.path) }}</code>
            <a-button
              v-if="canInteractWithPath(selectedEvent)"
              type="link"
              size="small"
              @click="previewRecordPath(selectedEvent)"
            >
              <template #icon><EyeOutlined /></template>
              Preview
            </a-button>
            <a-button
              v-if="canInteractWithPath(selectedEvent)"
              type="link"
              size="small"
              @click="openInExplorer(selectedEvent)"
            >
              <template #icon><FolderOpenOutlined /></template>
              Open in Explorer
            </a-button>
          </div>
        </a-descriptions-item>
      </a-descriptions>
    </a-modal>

    <FilePreviewDrawer
      v-model:open="showPreview"
      :loading="previewLoading"
      :preview="previewData"
      title="Log File Preview"
    />
  </div>
</template>

<style scoped>
.net-dir-bar {
  display: flex; align-items: center; gap: 8px;
  padding: 4px 0 10px;
  flex-wrap: wrap;
}
.dashboard-page {
  min-height: 280px;
  padding: 0;
  background: linear-gradient(180deg, #ffffff 0%, #f8fbf5 100%);
  font-family: Calibri, 'Segoe UI', Arial, sans-serif;
  color: #1f2937;
  width: 100%;
  box-sizing: border-box;
}

.dashboard-tabs {
  margin-bottom: 4px;
}

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
}

.dashboard-toolbar {
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

.excel-table {
  border: 1px solid #d9e4d1;
  border-radius: 6px;
  overflow: hidden;
  background: #fff;
  width: 100%;
  min-width: 100%;
}

.dashboard-table-wrap {
  width: 100%;
  min-width: 0;
  overflow-x: auto;
}

.excel-table :deep(.ant-table) {
  font-family: inherit;
  background: #fff;
  width: 100%;
}

.excel-table :deep(.ant-table-container) {
  border-top-left-radius: 6px;
  border-top-right-radius: 6px;
  width: 100%;
}

.excel-table :deep(.ant-table-thead > tr > th) {
  background: linear-gradient(180deg, #f7fbf4 0%, #edf4e8 100%);
  color: #1f3a1f;
  font-weight: 700;
  font-size: 13px;
  border-right: 1px solid #d9e4d1;
  border-bottom: 1px solid #c7d7bf;
  padding: 10px 14px;
  white-space: nowrap;
}

.excel-header-cell {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  width: 100%;
  min-width: 0;
}

.excel-header-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
  flex: 1 1 auto;
}

.excel-header-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex: 0 0 auto;
}

.excel-header-filter-trigger {
  flex: 0 0 auto;
  width: 20px !important;
  height: 20px !important;
  padding: 0 !important;
  border-radius: 2px !important;
  color: #5f7a52 !important;
}

.excel-header-filter-trigger.active {
  color: #2f7d32 !important;
  background: rgba(72, 143, 81, 0.12) !important;
}

.excel-column-resizer {
  width: 10px;
  align-self: stretch;
  flex: 0 0 auto;
  cursor: col-resize;
  position: relative;
  margin-left: 2px;
}

.excel-column-resizer::before {
  content: '';
  position: absolute;
  top: 18%;
  bottom: 18%;
  left: 4px;
  width: 1px;
  border-radius: 1px;
  background: rgba(95, 122, 82, 0.5);
}

.excel-column-resizer:hover::before {
  background: #2f7d32;
}

.excel-filter-dropdown {
  width: 240px;
  padding: 12px;
  border: 1px solid #d9e4d1;
  border-radius: 6px;
  background: #fff;
  box-shadow: 0 6px 18px rgba(34, 54, 24, 0.12);
}

.excel-filter-dropdown--wide {
  width: 420px;
}

.excel-filter-dropdown-title {
  font-size: 12px;
  font-weight: 700;
  color: #355238;
  margin-bottom: 10px;
  letter-spacing: 0.2px;
}

.excel-filter-dropdown-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 10px;
}

.excel-filter-dropdown :deep(.ant-select-selector) {
  min-height: 32px !important;
  height: auto !important;
  align-items: flex-start !important;
}

.excel-filter-dropdown :deep(.ant-select-selection-overflow) {
  flex-wrap: wrap;
  align-items: flex-start;
}

.excel-filter-dropdown :deep(.ant-select-selection-overflow-item) {
  margin-bottom: 2px;
}

.excel-path-cell {
  display: flex;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
}

.excel-type-cell {
  display: inline-flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}

.excel-occurrence-tag {
  font-size: 12px;
  line-height: 1;
  margin: 0;
}

.excel-path-text {
  flex: 1 1 auto;
  min-width: 0;
  display: block;
  color: #28402a;
  white-space: normal;
  word-break: break-word;
  overflow-wrap: anywhere;
  font-size: 13px;
  line-height: 1.5;
}

.excel-table :deep(.ant-table-thead > tr > th:last-child),
.excel-table :deep(.ant-table-tbody > tr > td:last-child) {
  border-right: none;
}

.excel-table :deep(.ant-table-tbody > tr > td) {
  border-right: 1px solid #e6ece0;
  border-bottom: 1px solid #e6ece0;
  padding: 10px 14px;
  background: #fff;
  vertical-align: top;
  min-width: 0;
  white-space: normal;
  word-break: break-word;
  overflow-wrap: anywhere;
  font-size: 13px;
  line-height: 1.5;
}

.excel-table :deep(.ant-table-tbody > tr.excel-row-even > td) {
  background: #ffffff;
}

.excel-table :deep(.ant-table-tbody > tr.excel-row-odd > td) {
  background: #fbfdf8;
}

.excel-table :deep(.ant-table-tbody > tr:hover > td) {
  background: #eef6e8 !important;
}

.excel-table :deep(.ant-table-row) {
  transition: background-color 0.15s ease;
}

.excel-table :deep(.ant-table-tbody > tr.excel-row-enter-top > td) {
  animation: excel-row-enter-top 320ms ease-out;
}

.excel-table :deep(.ant-table-tbody > tr.excel-row-enter-bottom > td) {
  animation: excel-row-enter-bottom 320ms ease-out;
}

.excel-table :deep(.ant-tag) {
  border-radius: 2px;
  font-weight: 600;
  letter-spacing: 0.1px;
}

.excel-table :deep(.ant-input),
.excel-table :deep(.ant-select-selector),
.excel-table :deep(.ant-btn),
.excel-table :deep(.ant-checkbox-inner) {
  border-radius: 2px !important;
}

.excel-table :deep(.ant-table-pagination) {
  margin: 12px 0 0;
}

.excel-table :deep(.ant-pagination-item),
.excel-table :deep(.ant-pagination-prev),
.excel-table :deep(.ant-pagination-next),
.excel-table :deep(.ant-select-selector) {
  box-shadow: none;
}

.excel-table :deep(.ant-dropdown) {
  z-index: 1200;
}

:global(html.excel-resizing),
:global(html.excel-resizing body),
:global(html.excel-resizing *) {
  cursor: col-resize !important;
  user-select: none !important;
}

@keyframes excel-row-enter-top {
  0% {
    opacity: 0;
    transform: translateY(-10px);
    background-color: #edf8e9;
  }
  70% {
    opacity: 1;
    transform: translateY(0);
    background-color: #f3fbef;
  }
  100% {
    opacity: 1;
    transform: translateY(0);
    background-color: inherit;
  }
}

@keyframes excel-row-enter-bottom {
  0% {
    opacity: 0;
    transform: translateY(10px);
    background-color: #edf8e9;
  }
  70% {
    opacity: 1;
    transform: translateY(0);
    background-color: #f3fbef;
  }
  100% {
    opacity: 1;
    transform: translateY(0);
    background-color: inherit;
  }
}
</style>
