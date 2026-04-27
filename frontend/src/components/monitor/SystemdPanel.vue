<script setup lang="ts">
import type { SystemdService } from '../../composables/useSystemd';

defineProps<{
  systemdServices: SystemdService[];
  systemdLoading: boolean;
  filteredSystemdServices: SystemdService[];
  systemdScope: 'system' | 'user';
  systemdSearch: string;
  systemdColumns: any[];
  showLogsModal: boolean;
  activeLogUnit: string;
  serviceLogs: string;
  logsLoading: boolean;
}>();

const emit = defineEmits<{
  'update:systemdScope': [value: 'system' | 'user'];
  'update:systemdSearch': [value: string];
  'update:showLogsModal': [value: boolean];
  refresh: [];
  control: [unit: string, action: string];
  fetchLogs: [unit: string];
}>();
</script>

<template>
  <div style="background: #fff; padding: 20px; border-radius: 4px; border: 1px solid #f0f0f0;">
    <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center; gap: 16px; flex-wrap: wrap;">
      <a-space>
        <a-radio-group :value="systemdScope" @change="(e: any) => emit('update:systemdScope', e?.target?.value ?? e)" button-style="solid" size="small">
          <a-radio-button value="system">System</a-radio-button>
          <a-radio-button value="user">User</a-radio-button>
        </a-radio-group>
        <a-input-search :value="systemdSearch" @update:value="emit('update:systemdSearch', $event)" placeholder="Filter services..." style="width: 260px" size="small" allow-clear />
      </a-space>
      <a-button type="primary" size="small" :loading="systemdLoading" @click="emit('refresh')">Refresh</a-button>
    </div>
    <a-table :dataSource="filteredSystemdServices" :columns="systemdColumns" row-key="unit" size="small" :pagination="{ pageSize: 50, showSizeChanger: true }" :loading="systemdLoading" :scroll="{ x: 800 }">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'active'"><a-tag :color="record.active === 'active' ? 'success' : 'default'">{{ record.active }}</a-tag></template>
        <template v-else-if="column.key === 'action'">
          <a-space>
            <a-button type="link" size="small" @click="emit('fetchLogs', record.unit)">Logs</a-button>
            <a-button v-if="record.active !== 'active'" type="link" size="small" @click="emit('control', record.unit, 'start')">Start</a-button>
            <a-button v-if="record.active === 'active'" type="link" size="small" danger @click="emit('control', record.unit, 'stop')">Stop</a-button>
            <a-button type="link" size="small" @click="emit('control', record.unit, 'restart')">Restart</a-button>
          </a-space>
        </template>
      </template>
    </a-table>

    <a-modal :open="showLogsModal" :title="`Logs: ${activeLogUnit}`" width="1000px" :footer="null" @update:open="emit('update:showLogsModal', $event)">
      <div style="background:#1e1e1e;color:#d4d4d4;padding:12px;border-radius:4px;font-family:'JetBrains Mono',monospace;font-size:13px;max-height:600px;overflow-y:auto;white-space:pre-wrap;word-break:break-all;">
        <a-spin :spinning="logsLoading">
          <div v-if="serviceLogs">{{ serviceLogs }}</div>
          <a-empty v-else-if="!logsLoading" description="No logs" />
        </a-spin>
      </div>
    </a-modal>
  </div>
</template>
