<script setup lang="ts">
import { ref } from 'vue';
import { ClusterOutlined, DeleteOutlined, DownloadOutlined, ReloadOutlined, SearchOutlined, UploadOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import AgentSightLogView from './AgentSightLogView.vue';
import AgentSightMetricsView from './AgentSightMetricsView.vue';
import AgentSightProcessTreeView from './AgentSightProcessTreeView.vue';
import AgentSightTimelineView from './AgentSightTimelineView.vue';
import { useAgentSightI18n } from '../../composables/agentsight/useAgentSightI18n';
import { useAgentSightEvents } from '../../composables/agentsight/useAgentSightEvents';

const state = useAgentSightEvents();
const { locale, localeOptions, t, setLocale } = useAgentSightI18n();
const pasteText = ref('');

const tabs = [
  { key: 'process-tree', label: 'Process Tree' },
  { key: 'timeline', label: 'Timeline' },
  { key: 'log', label: 'Log' },
  { key: 'metrics', label: 'Metrics' },
];

const limitOptions = [
  { label: '500', value: 500 },
  { label: '2,000', value: 2000 },
  { label: '10,000', value: 10000 },
  { label: '全部', value: 0 },
];

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

const loadSampleDemo = async () => {
  const count = await state.loadSampleTrace();
  if (count > 0) message.success(`Loaded ${count} bundled AgentSight sample events`);
  else message.warning('Bundled AgentSight sample trace is unavailable');
};
</script>

<template>
  <div class="agentsight-panel">
    <a-card :bordered="false" class="agentsight-shell">
      <template #title>
        <span class="agentsight-title"><ClusterOutlined /> 行为追踪</span>
      </template>
      <template #extra>
        <a-space wrap>
          <a-select :value="locale" size="small" style="width: 110px" :options="localeOptions" @change="changeLocale" />
          <a-badge :status="state.isEnvelopeConnected.value ? 'success' : 'error'" :text="state.isEnvelopeConnected.value ? 'Envelope live' : 'Envelope offline'" />
          <a-badge :status="state.isTLSConnected.value ? 'success' : 'warning'" :text="state.isTLSConnected.value ? 'TLS live' : 'TLS offline'" />
          <a-badge :status="state.isSystemConnected.value ? 'success' : 'warning'" :text="state.isSystemConnected.value ? 'System live' : 'System offline'" />
          <a-select v-model:value="state.limit.value" size="small" style="width: 110px" :options="limitOptions" />
          <a-tag color="purple">{{ state.metrics.value.total }} {{ t.events }}</a-tag>
          <a-button size="small" :loading="state.loading.value || state.tlsLoading.value" @click="state.fetchEvents">
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
        class="agentsight-alert"
      />

      <a-row :gutter="[12, 12]" class="summary-row">
        <a-col :xs="12" :md="3"><a-card size="small"><a-statistic title="Total" :value="state.metrics.value.total" /></a-card></a-col>
        <a-col :xs="12" :md="3"><a-card size="small"><a-statistic title="TLS" :value="state.metrics.value.tls" /></a-card></a-col>
        <a-col :xs="12" :md="3"><a-card size="small"><a-statistic title="HTTP" :value="state.metrics.value.http" /></a-card></a-col>
        <a-col :xs="12" :md="3"><a-card size="small"><a-statistic title="SSE" :value="state.metrics.value.sse" /></a-card></a-col>
        <a-col :xs="12" :md="3"><a-card size="small"><a-statistic title="Stdio/MCP" :value="state.metrics.value.stdio" /></a-card></a-col>
        <a-col :xs="12" :md="3"><a-card size="small"><a-statistic title="Alerts" :value="state.metrics.value.alerts" /></a-card></a-col>
        <a-col :xs="12" :md="3"><a-card size="small"><a-statistic title="Processes" :value="state.metrics.value.processes" /></a-card></a-col>
        <a-col :xs="12" :md="3"><a-card size="small"><a-statistic title="System" :value="state.metrics.value.system" /></a-card></a-col>
      </a-row>

      <a-card size="small" :title="t.traceImport" class="agentsight-import">
        <a-space direction="vertical" style="width: 100%">
          <a-space wrap>
            <a-upload :before-upload="beforeUploadTrace" :show-upload-list="false" accept=".json,.jsonl,.log,.txt">
              <a-button size="small"><template #icon><UploadOutlined /></template>{{ t.uploadTrace }}</a-button>
            </a-upload>
            <a-tag color="blue">{{ state.importedRecords.value.length }} {{ t.importedRecords }}</a-tag>
            <a-tag v-if="state.metrics.value.sample" color="gold">{{ state.metrics.value.sample }} sample events</a-tag>
            <a-button size="small" danger @click="state.clearImportedRecords">{{ t.clearImported }}</a-button>
            <a-button size="small" :loading="state.sampleLoading.value" @click="loadSampleDemo">Load bundled demo</a-button>
          </a-space>
          <a-textarea v-model:value="pasteText" :rows="3" :placeholder="t.pastePlaceholder" />
          <a-space wrap>
            <a-button size="small" type="primary" :disabled="!pasteText.trim()" @click="importPastedRecords">{{ t.importPasted }}</a-button>
            <a-button size="small" @click="state.exportVisibleJSON">
              <template #icon><DownloadOutlined /></template>
              Export JSON
            </a-button>
            <a-button size="small" @click="state.exportVisibleJSONL">
              <template #icon><DownloadOutlined /></template>
              Export JSONL
            </a-button>
            <a-button size="small" @click="state.exportVisibleCSV">
              <template #icon><DownloadOutlined /></template>
              Export CSV
            </a-button>
            <a-button size="small" danger @click="state.clearAllRecords">
              <template #icon><DeleteOutlined /></template>
              Clear all local data
            </a-button>
          </a-space>
        </a-space>
      </a-card>

      <a-card size="small" class="agentsight-filters">
        <a-row :gutter="[12, 12]">
          <a-col :xs="24" :md="5">
            <a-input v-model:value="state.filters.value.searchTerm" size="small" placeholder="Search everything" allow-clear>
              <template #prefix><SearchOutlined /></template>
            </a-input>
          </a-col>
          <a-col :xs="24" :md="3">
            <a-input v-model:value="state.filters.value.comm" size="small" :placeholder="t.command" allow-clear />
          </a-col>
          <a-col :xs="24" :md="2">
            <a-input v-model:value="state.filters.value.pid" size="small" :placeholder="t.pid" allow-clear />
          </a-col>
          <a-col :xs="24" :md="3">
            <a-input v-model:value="state.filters.value.traceId" size="small" :placeholder="t.traceId" allow-clear />
          </a-col>
          <a-col :xs="24" :md="3">
            <a-select v-model:value="state.filters.value.source" size="small" allow-clear :placeholder="t.source" style="width: 100%" :options="state.sourceOptions.value" />
          </a-col>
          <a-col :xs="24" :md="3">
            <a-select v-model:value="state.filters.value.eventType" size="small" allow-clear :placeholder="t.eventType" style="width: 100%" :options="state.eventTypeOptions.value" />
          </a-col>
          <a-col :xs="24" :md="3">
            <a-select v-model:value="state.filters.value.redactionState" size="small" allow-clear :placeholder="t.redaction" style="width: 100%" :options="state.redactionStateOptions.value" />
          </a-col>
          <a-col :xs="24" :md="2">
            <a-button size="small" block @click="state.clearFilters">{{ t.clear }}</a-button>
          </a-col>
        </a-row>
      </a-card>

      <a-tabs v-model:activeKey="state.activeTab.value">
        <a-tab-pane v-for="tab in tabs" :key="tab.key" :tab="tab.label">
          <AgentSightProcessTreeView v-if="tab.key === 'process-tree'" :events="state.visibleEvents.value" />
          <AgentSightTimelineView v-else-if="tab.key === 'timeline'" :events="state.visibleProcessedEvents.value" />
          <AgentSightLogView v-else-if="tab.key === 'log'" :events="state.visibleProcessedEvents.value" />
          <AgentSightMetricsView v-else :events="state.visibleEvents.value" />
        </a-tab-pane>
      </a-tabs>
    </a-card>
  </div>
</template>

<style scoped>
.agentsight-panel {
  min-width: 0;
}

.agentsight-shell {
  border-radius: 14px;
}

.agentsight-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 700;
}

.agentsight-alert,
.summary-row,
.agentsight-import,
.agentsight-filters {
  margin-bottom: 16px;
}

.summary-row :deep(.ant-statistic-title) {
  color: #64748b;
  font-size: 12px;
}

.summary-row :deep(.ant-statistic-content) {
  color: #0f172a;
  font-size: 20px;
  font-weight: 700;
}
</style>
