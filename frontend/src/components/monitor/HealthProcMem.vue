<script setup lang="ts">
import { computed } from 'vue';

const props = defineProps<{
  processes: any[];
  formatBytesWithUnit: (bytes: number) => string;
}>();

interface ProcGroup {
  name: string;
  count: number;
  totalMem: number;
  pids: number[];
  instances: typeof props.processes;
}

const groupedProcesses = computed(() => {
  const groups: Record<string, ProcGroup> = {};
  for (const p of props.processes) {
    const name = p.name || 'unknown';
    if (!groups[name]) {
      groups[name] = { name, count: 0, totalMem: 0, pids: [], instances: [] };
    }
    groups[name].count++;
    groups[name].totalMem += Number(p.mem || 0);
    groups[name].pids.push(p.pid);
    groups[name].instances.push(p);
  }
  return Object.values(groups).sort((a, b) => b.totalMem - a.totalMem);
});

const totalMemAll = computed(() => groupedProcesses.value.reduce((sum, g) => sum + g.totalMem, 0));

const columns = [
  { title: 'Process', dataIndex: 'name', key: 'name', width: 200 },
  { title: 'Instances', dataIndex: 'count', key: 'count', width: 80, align: 'right' as const },
  { title: 'Memory', dataIndex: 'totalMem', key: 'totalMem', width: 140 },
  { title: 'Share', key: 'share', width: 120 },
  { title: 'PIDs', key: 'pids', ellipsis: true },
];

const maxMem = computed(() => {
  const m = groupedProcesses.value[0]?.totalMem || 0;
  return m > 0 ? m : 1;
});
</script>

<template>
  <div style="padding-top: 16px;">
    <div style="margin-bottom: 12px; display: flex; justify-content: space-between; align-items: center;">
      <span style="font-weight: 600;">Process Memory Usage ({{ groupedProcesses.length }} unique)</span>
      <span style="color: #888; font-size: 12px;">Total: {{ formatBytesWithUnit(totalMemAll) }}</span>
    </div>
    <a-table
      :dataSource="groupedProcesses"
      :columns="columns"
      row-key="name"
      size="small"
      :pagination="{ pageSize: 30, showSizeChanger: true, pageSizeOptions: ['20', '30', '50', '100'] }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'">
          <span style="font-weight: 500;">{{ record.name }}</span>
        </template>
        <template v-if="column.key === 'count'">
          <a-tag :color="record.count > 10 ? 'red' : record.count > 5 ? 'orange' : 'blue'">{{ record.count }}</a-tag>
        </template>
        <template v-if="column.key === 'totalMem'">
          <div>
            <span style="font-weight: 500;">{{ formatBytesWithUnit(record.totalMem) }}</span>
            <a-progress
              :percent="Math.round((record.totalMem / maxMem) * 100)"
              :show-info="false"
              size="small"
              :stroke-color="record.totalMem / totalMemAll > 0.2 ? '#ff4d4f' : '#1890ff'"
              style="margin-top: 2px;"
            />
          </div>
        </template>
        <template v-if="column.key === 'share'">
          <span style="font-size: 12px; color: #555;">{{ totalMemAll > 0 ? ((record.totalMem / totalMemAll) * 100).toFixed(1) : 0 }}%</span>
        </template>
        <template v-if="column.key === 'pids'">
          <span style="font-family: monospace; font-size: 11px;">{{ record.pids.slice(0, 15).join(', ') }}{{ record.pids.length > 15 ? '...' : '' }}</span>
        </template>
      </template>
    </a-table>
  </div>
</template>
