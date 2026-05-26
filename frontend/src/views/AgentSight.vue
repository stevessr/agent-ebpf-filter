<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ClusterOutlined, ReloadOutlined, SearchOutlined, UploadOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import { useAgentSightI18n } from '../composables/useAgentSightI18n';
import {
  formatAgentSightTime,
  recordComm,
  recordEventType,
  recordPID,
  recordRedactionState,
  recordSource,
  recordTitle,
  recordTraceID,
  useAgentSightEvents,
} from '../composables/useAgentSightEvents';

const route = useRoute();
const router = useRouter();
const state = useAgentSightEvents();
const { locale, localeOptions, t, setLocale } = useAgentSightI18n();
const pasteText = ref('');
const timelineZoom = ref(100);
const timelineWindow = ref<[number, number]>([0, 100]);

const tabs = [
  { key: 'log', labelKey: 'log' },
  { key: 'timeline', labelKey: 'timeline' },
  { key: 'process-tree', labelKey: 'processTree' },
  { key: 'metrics', labelKey: 'metrics' },
];

const syncTabFromRoute = () => {
  const tab = String(route.params.tab || 'log');
  state.activeTab.value = tabs.some(item => item.key === tab) ? tab : 'log';
};

watch(() => route.params.tab, syncTabFromRoute, { immediate: true });

const changeTab = (key: string) => {
  router.push({ name: 'AgentSight', params: { tab: key } });
};

const changeLocale = (value: string | number) => {
  setLocale(value === 'zh' ? 'zh' : 'en');
};

const importPastedRecords = () => {
  const count = state.importRecordsText(pasteText.value);
  pasteText.value = '';
  message.success(`Imported ${count} records`);
};

const beforeUploadTrace = async (file: File) => {
  const text = await file.text();
  const count = state.importRecordsText(text);
  message.success(`Imported ${count} records from ${file.name}`);
  return false;
};

const timelineBounds = computed(() => {
  const times = state.records.value.map(record => Number(record.Timestamp || 0)).filter(Boolean);
  if (times.length === 0) return { min: 0, max: 0, span: 1 };
  const min = Math.min(...times);
  const max = Math.max(...times);
  return { min, max, span: Math.max(max - min, 1) };
});

const timelineRows = computed(() => state.records.value.map(record => ({
  time: Number(record.Timestamp || 0),
  source: recordSource(record),
  type: recordEventType(record),
  title: recordTitle(record),
  pid: recordPID(record),
  comm: recordComm(record),
  trace: recordTraceID(record),
  lane: recordSource(record),
  offset: timelineBounds.value.span > 0 ? ((Number(record.Timestamp || 0) - timelineBounds.value.min) / timelineBounds.value.span) * 100 : 0,
})).sort((a, b) => a.time - b.time));

const visibleTimelineRows = computed(() => {
  const [start, end] = timelineWindow.value;
  return timelineRows.value.filter(row => row.offset >= start && row.offset <= end);
});

const timelineLanes = computed(() => Array.from(new Set(visibleTimelineRows.value.map(row => row.lane))).sort());

const timelineLaneRows = computed(() => timelineLanes.value.map(lane => ({
  lane,
  rows: visibleTimelineRows.value.filter(row => row.lane === lane),
})));
</script>

<template>
  <div class="agentsight-page">
    <a-card :bordered="false">
      <template #title>
        <span class="agentsight-title"><ClusterOutlined /> {{ t.title }}</span>
      </template>
      <template #extra>
        <a-space>
          <a-select :value="locale" size="small" style="width: 110px" :options="localeOptions" @change="changeLocale" />
          <a-badge :status="state.isConnected.value ? 'success' : 'error'" :text="state.isConnected.value ? 'Live' : 'Offline'" />
          <a-tag color="purple">{{ state.metrics.value.total }} {{ t.events }}</a-tag>
          <a-button size="small" :loading="state.loading.value" @click="state.fetchEvents">
            <template #icon><ReloadOutlined /></template>
            {{ t.refresh }}
          </a-button>
        </a-space>
      </template>

      <a-alert
        type="info"
        show-icon
        :message="t.compatMessage"
        :description="t.compatDescription"
        style="margin-bottom: 16px"
      />

      <a-card size="small" :title="t.traceImport" class="agentsight-import">
        <a-space direction="vertical" style="width: 100%">
          <a-space wrap>
            <a-upload :before-upload="beforeUploadTrace" :show-upload-list="false" accept=".json,.jsonl,.log,.txt">
              <a-button size="small"><template #icon><UploadOutlined /></template>{{ t.uploadTrace }}</a-button>
            </a-upload>
            <a-tag color="blue">{{ state.importedRecords.value.length }} {{ t.importedRecords }}</a-tag>
            <a-button size="small" danger @click="state.clearImportedRecords">{{ t.clearImported }}</a-button>
          </a-space>
          <a-textarea v-model:value="pasteText" :rows="3" :placeholder="t.pastePlaceholder" />
          <a-button size="small" type="primary" :disabled="!pasteText.trim()" @click="importPastedRecords">{{ t.importPasted }}</a-button>
        </a-space>
      </a-card>

      <a-row :gutter="[12, 12]" class="agentsight-filters">
        <a-col :xs="24" :md="4">
          <a-input v-model:value="state.filters.value.comm" size="small" :placeholder="t.command" allow-clear>
            <template #prefix><SearchOutlined /></template>
          </a-input>
        </a-col>
        <a-col :xs="24" :md="3">
          <a-input v-model:value="state.filters.value.pid" size="small" :placeholder="t.pid" allow-clear />
        </a-col>
        <a-col :xs="24" :md="5">
          <a-input v-model:value="state.filters.value.traceId" size="small" :placeholder="t.traceId" allow-clear />
        </a-col>
        <a-col :xs="24" :md="4">
          <a-select v-model:value="state.filters.value.source" size="small" allow-clear :placeholder="t.source" style="width: 100%" :options="state.sourceOptions.value" />
        </a-col>
        <a-col :xs="24" :md="4">
          <a-select v-model:value="state.filters.value.eventType" size="small" allow-clear :placeholder="t.eventType" style="width: 100%" :options="state.eventTypeOptions.value" />
        </a-col>
        <a-col :xs="24" :md="3">
          <a-select v-model:value="state.filters.value.redactionState" size="small" allow-clear :placeholder="t.redaction" style="width: 100%" :options="[
            { label: 'Sanitized', value: 'sanitized' },
            { label: 'Raw/none', value: 'raw' },
          ]" />
        </a-col>
        <a-col :xs="24" :md="1">
          <a-button size="small" @click="state.clearFilters">{{ t.clear }}</a-button>
        </a-col>
      </a-row>

      <a-tabs :active-key="state.activeTab.value" @change="(key: string | number) => changeTab(String(key))">
        <a-tab-pane key="log" tab="Log">
          <a-table :data-source="state.records.value" :loading="state.loading.value" size="small" :pagination="{ pageSize: 20, showSizeChanger: true }" :scroll="{ x: 1200 }" row-key="Timestamp">
            <a-table-column title="Time" data-index="Timestamp" key="time" width="180">
              <template #default="{ text }">{{ formatAgentSightTime(text) }}</template>
            </a-table-column>
            <a-table-column title="Source" key="source" width="140">
              <template #default="{ record }"><a-tag>{{ recordSource(record) }}</a-tag></template>
            </a-table-column>
            <a-table-column title="Type" key="type" width="160">
              <template #default="{ record }"><a-tag color="geekblue">{{ recordEventType(record) }}</a-tag></template>
            </a-table-column>
            <a-table-column title="PID" key="pid" width="90">
              <template #default="{ record }">{{ recordPID(record) || '—' }}</template>
            </a-table-column>
            <a-table-column title="Command" key="comm" width="160" ellipsis>
              <template #default="{ record }">{{ recordComm(record) }}</template>
            </a-table-column>
            <a-table-column title="Trace" key="trace" width="180" ellipsis>
              <template #default="{ record }"><a-typography-text code>{{ recordTraceID(record) || '—' }}</a-typography-text></template>
            </a-table-column>
            <a-table-column title="Redaction" key="redaction" width="120">
              <template #default="{ record }"><a-tag :color="recordRedactionState(record) === 'sanitized' ? 'green' : 'default'">{{ recordRedactionState(record) || '—' }}</a-tag></template>
            </a-table-column>
            <a-table-column title="Summary" key="summary" ellipsis>
              <template #default="{ record }">{{ recordTitle(record) }}</template>
            </a-table-column>
          </a-table>
        </a-tab-pane>

        <a-tab-pane key="timeline" tab="Timeline">
          <a-card size="small" class="timeline-controls">
            <a-row :gutter="[16, 12]" align="middle">
              <a-col :xs="24" :md="8">
                <span>Zoom: </span>
                <a-slider v-model:value="timelineZoom" :min="25" :max="400" :step="25" />
              </a-col>
              <a-col :xs="24" :md="10">
                <span>Window: </span>
                <a-slider v-model:value="timelineWindow" range :min="0" :max="100" />
              </a-col>
              <a-col :xs="24" :md="6">
                <a-tag>{{ visibleTimelineRows.length }} visible</a-tag>
                <a-tag>{{ timelineLanes.length }} lanes</a-tag>
              </a-col>
            </a-row>
            <div class="timeline-minimap">
              <span
                v-for="row in timelineRows"
                :key="`mini-${row.time}-${row.pid}-${row.type}`"
                class="timeline-minimap-dot"
                :style="{ left: `${row.offset}%`, background: row.type.includes('ALERT') ? '#ff4d4f' : row.type.includes('TLS') || row.type.includes('HTTP') ? '#1677ff' : '#8c8c8c' }"
              />
              <div class="timeline-window" :style="{ left: `${timelineWindow[0]}%`, width: `${timelineWindow[1] - timelineWindow[0]}%` }" />
            </div>
          </a-card>

          <div class="timeline-lanes" :style="{ minWidth: `${timelineZoom}%` }">
            <div v-for="lane in timelineLaneRows" :key="lane.lane" class="timeline-lane">
              <div class="timeline-lane-label"><a-tag>{{ lane.lane }}</a-tag></div>
              <div class="timeline-lane-track">
                <div
                  v-for="row in lane.rows"
                  :key="`${row.time}-${row.pid}-${row.type}-${row.title}`"
                  class="timeline-event"
                  :style="{ left: `${row.offset}%` }"
                >
                  <a-tooltip>
                    <template #title>{{ formatAgentSightTime(row.time) }} · {{ row.comm }}#{{ row.pid || '—' }} · {{ row.title }}</template>
                    <span :class="['timeline-event-dot', { alert: row.type.includes('ALERT'), tls: row.type.includes('TLS') || row.type.includes('HTTP') }]" />
                  </a-tooltip>
                </div>
              </div>
            </div>
          </div>

          <a-timeline style="margin-top: 16px">
            <a-timeline-item v-for="row in visibleTimelineRows" :key="`${row.time}-${row.pid}-${row.type}-${row.title}`" :color="row.type.includes('ALERT') ? 'red' : row.type.includes('TLS') || row.type.includes('HTTP') ? 'blue' : 'gray'">
              <div class="timeline-row">
                <a-typography-text type="secondary">{{ formatAgentSightTime(row.time) }}</a-typography-text>
                <a-tag>{{ row.source }}</a-tag>
                <a-tag color="geekblue">{{ row.type }}</a-tag>
                <span>{{ row.comm }}#{{ row.pid || '—' }}</span>
                <strong>{{ row.title }}</strong>
              </div>
            </a-timeline-item>
          </a-timeline>
        </a-tab-pane>

        <a-tab-pane key="process-tree" tab="Process Tree">
          <a-table :data-source="state.processTree.value" size="small" :pagination="false" row-key="pid">
            <a-table-column title="PID" data-index="pid" key="pid" width="120" />
            <a-table-column title="PPID" data-index="ppid" key="ppid" width="120" />
            <a-table-column title="Command" data-index="comm" key="comm" />
            <a-table-column title="Events" data-index="events" key="events" width="120" />
            <a-table-column title="Children" key="children" width="180">
              <template #default="{ record }">
                <a-space wrap><a-tag v-for="child in record.children" :key="child">{{ child }}</a-tag></a-space>
              </template>
            </a-table-column>
          </a-table>
        </a-tab-pane>

        <a-tab-pane key="metrics" tab="Metrics">
          <a-row :gutter="[16, 16]">
            <a-col :xs="12" :md="6"><a-card size="small"><a-statistic title="Total" :value="state.metrics.value.total" /></a-card></a-col>
            <a-col :xs="12" :md="6"><a-card size="small"><a-statistic title="TLS" :value="state.metrics.value.tls" /></a-card></a-col>
            <a-col :xs="12" :md="6"><a-card size="small"><a-statistic title="HTTP" :value="state.metrics.value.http" /></a-card></a-col>
            <a-col :xs="12" :md="6"><a-card size="small"><a-statistic title="SSE" :value="state.metrics.value.sse" /></a-card></a-col>
            <a-col :xs="12" :md="6"><a-card size="small"><a-statistic title="Sanitized" :value="state.metrics.value.sanitized" /></a-card></a-col>
            <a-col :xs="12" :md="6"><a-card size="small"><a-statistic title="Alerts" :value="state.metrics.value.alerts" /></a-card></a-col>
            <a-col :xs="12" :md="6"><a-card size="small"><a-statistic title="Processes" :value="state.metrics.value.processes" /></a-card></a-col>
          </a-row>
        </a-tab-pane>
      </a-tabs>
    </a-card>
  </div>
</template>

<style scoped>
.agentsight-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.agentsight-import,
.agentsight-filters {
  margin-bottom: 16px;
}

.timeline-controls {
  margin-bottom: 16px;
}

.timeline-minimap {
  position: relative;
  height: 28px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #f8fafc;
  overflow: hidden;
}

.timeline-minimap-dot {
  position: absolute;
  top: 9px;
  width: 4px;
  height: 10px;
  border-radius: 2px;
}

.timeline-window {
  position: absolute;
  top: 0;
  bottom: 0;
  border: 1px solid #1677ff;
  background: rgba(22, 119, 255, 0.12);
}

.timeline-lanes {
  overflow-x: auto;
}

.timeline-lane {
  display: grid;
  grid-template-columns: 150px 1fr;
  align-items: center;
  min-height: 40px;
  border-bottom: 1px solid #f0f0f0;
}

.timeline-lane-track {
  position: relative;
  height: 32px;
}

.timeline-event {
  position: absolute;
  top: 9px;
}

.timeline-event-dot {
  display: block;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #8c8c8c;
}

.timeline-event-dot.tls {
  background: #1677ff;
}

.timeline-event-dot.alert {
  background: #ff4d4f;
}

.timeline-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
</style>
