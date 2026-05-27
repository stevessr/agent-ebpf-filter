<script setup lang="ts">
import { computed, ref } from 'vue';
import type { ProcessInfo } from '../composables/useMonitorData';

type ProcessRow = ProcessInfo & {
  key: number;
  children?: ProcessRow[];
};

const props = withDefaults(defineProps<{
  open: boolean;
  processes: ProcessInfo[];
  selectedPid?: number | null;
  loading?: boolean;
  title?: string;
}>(), {
  selectedPid: null,
  loading: false,
  title: '选择进程',
});

const emit = defineEmits<{
  (event: 'update:open', value: boolean): void;
  (event: 'select', process: ProcessInfo): void;
}>();

const processSearch = ref('');
const viewMode = ref<'flat' | 'tree'>('flat');

const columns = [
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 92, sorter: (a: ProcessRow, b: ProcessRow) => a.pid - b.pid },
  { title: 'Name', dataIndex: 'name', key: 'name', sorter: (a: ProcessRow, b: ProcessRow) => a.name.localeCompare(b.name) },
  { title: 'PPID', dataIndex: 'ppid', key: 'ppid', width: 92, sorter: (a: ProcessRow, b: ProcessRow) => a.ppid - b.ppid },
  { title: 'CPU', dataIndex: 'cpu', key: 'cpu', width: 88, align: 'right' as const, sorter: (a: ProcessRow, b: ProcessRow) => (a.cpu ?? 0) - (b.cpu ?? 0), defaultSortOrder: 'descend' as const },
  { title: 'Mem', dataIndex: 'mem', key: 'mem', width: 88, align: 'right' as const, sorter: (a: ProcessRow, b: ProcessRow) => (a.mem ?? 0) - (b.mem ?? 0) },
  { title: 'User', dataIndex: 'user', key: 'user', width: 120, sorter: (a: ProcessRow, b: ProcessRow) => (a.user || '').localeCompare(b.user || '') },
  { title: 'Command Line', dataIndex: 'cmdline', key: 'cmdline' },
  { title: 'Action', key: 'action', width: 96, align: 'right' as const },
];

const baseRows = computed<ProcessRow[]>(() => props.processes.map((process) => ({
  ...process,
  cpu: process.cpu ?? 0,
  mem: process.mem ?? 0,
  key: process.pid,
})));

const filteredRows = computed(() => {
  const query = processSearch.value.trim().toLowerCase();
  const list = query
    ? baseRows.value.filter((process) => (
      process.name.toLowerCase().includes(query) ||
      String(process.pid).includes(query) ||
      String(process.ppid).includes(query) ||
      (process.cmdline ?? '').toLowerCase().includes(query) ||
      (process.user ?? '').toLowerCase().includes(query)
    ))
    : baseRows.value;
  return [...list].sort((a, b) => (b.cpu ?? 0) - (a.cpu ?? 0) || a.pid - b.pid);
});

const tableRows = computed<ProcessRow[]>(() => {
  if (viewMode.value !== 'tree') {
    return filteredRows.value;
  }

  const map: Record<number, ProcessRow> = {};
  filteredRows.value.forEach((process) => {
    map[process.pid] = { ...process, children: [] };
  });

  const roots: ProcessRow[] = [];
  filteredRows.value.forEach((process) => {
    const row = map[process.pid];
    const parent = map[process.ppid];
    if (parent && process.ppid !== process.pid) {
      parent.children!.push(row);
    } else {
      roots.push(row);
    }
  });
  return roots;
});

const close = () => {
  emit('update:open', false);
};

const selectProcess = (process: ProcessInfo) => {
  emit('select', process);
  close();
};
</script>

<template>
  <a-modal
    :open="open"
    :title="title"
    width="1080px"
    :footer="null"
    destroy-on-close
    @cancel="close"
  >
    <a-space direction="vertical" size="middle" style="width: 100%;">
      <div class="process-picker-toolbar">
        <a-space wrap>
          <a-radio-group v-model:value="viewMode" button-style="solid" size="small">
            <a-radio-button value="flat">Flat</a-radio-button>
            <a-radio-button value="tree">Tree</a-radio-button>
          </a-radio-group>
          <a-input-search
            v-model:value="processSearch"
            allow-clear
            placeholder="搜索 name / PID / PPID / user / cmdline"
            style="width: 360px;"
          />
        </a-space>
        <a-space>
          <a-tag v-if="selectedPid" color="processing">当前 PID {{ selectedPid }}</a-tag>
          <a-tag>{{ filteredRows.length }} / {{ processes.length }} processes</a-tag>
        </a-space>
      </div>

      <a-table
        :data-source="tableRows"
        :columns="columns"
        :loading="loading"
        size="small"
        row-key="pid"
        :pagination="{ pageSize: 12, showSizeChanger: true, pageSizeOptions: ['12', '25', '50', '100'] }"
        :scroll="{ y: 520, x: 980 }"
        :row-class-name="(record: ProcessRow) => record.pid === selectedPid ? 'process-picker-row-selected' : ''"
      >
        <template #bodyCell="{ column, record, text }">
          <template v-if="column.key === 'pid'">
            <code>{{ text }}</code>
          </template>
          <template v-else-if="column.key === 'name'">
            <a-button type="link" size="small" style="padding: 0;" @click="selectProcess(record)">
              {{ text || 'process' }}
            </a-button>
          </template>
          <template v-else-if="column.key === 'cpu' || column.key === 'mem'">
            {{ Number(text ?? 0).toFixed(1) }}%
          </template>
          <template v-else-if="column.key === 'cmdline'">
            <a-typography-text class="process-picker-cmdline" :title="text || ''">
              {{ text || '—' }}
            </a-typography-text>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button type="primary" size="small" @click="selectProcess(record)">选择</a-button>
          </template>
        </template>
      </a-table>
    </a-space>
  </a-modal>
</template>

<style scoped>
.process-picker-toolbar {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  align-items: center;
}

.process-picker-cmdline {
  display: inline-block;
  max-width: 420px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.process-picker-row-selected) > td {
  background: #e6f4ff !important;
}
</style>
