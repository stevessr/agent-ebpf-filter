<script setup lang="ts">
import type { ProcessInfo } from '../../composables/useMonitorData';

defineProps<{
  trackedProcesses: ProcessInfo[];
  trackedCommsNames: string[];
  sendProcessSignal: (pid: number, signal: string) => void;
}>();

const emit = defineEmits<{
  refresh: [];
}>();

const columns = [
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 100, sorter: (a: any, b: any) => a.pid - b.pid },
  { title: 'Name', dataIndex: 'name', key: 'name', sorter: (a: any, b: any) => a.name.localeCompare(b.name) },
  { title: 'CPU %', dataIndex: 'cpu', key: 'cpu', width: 90, align: 'right', sorter: (a: any, b: any) => a.cpu - b.cpu },
  { title: 'Mem %', dataIndex: 'mem', key: 'mem', width: 90, align: 'right', sorter: (a: any, b: any) => a.mem - b.mem },
  { title: 'GPU Util', dataIndex: 'gpuUtil', key: 'gpuUtil', width: 90, align: 'right', sorter: (a: any, b: any) => a.gpuUtil - b.gpuUtil },
  { title: 'VRAM', dataIndex: 'gpuMem', key: 'gpuMem', width: 90, align: 'right', sorter: (a: any, b: any) => a.gpuMem - b.gpuMem },
  { title: 'User', dataIndex: 'user', key: 'user', width: 100 },
  { title: 'Action', key: 'action', width: 260, align: 'right' },
];

</script>

<template>
  <div style="background:#fff;padding:20px;border-radius:4px;">
    <div style="margin-bottom:16px;display:flex;justify-content:space-between;align-items:center;">
      <div style="display:flex;gap:8px;">
        <span style="font-weight:bold;">Tracked:</span>
        <a-tag v-for="name in trackedCommsNames" :key="name" color="blue">{{ name }}</a-tag>
        <span v-if="trackedCommsNames.length === 0" style="color:#888;">No tracked processes</span>
      </div>
      <a-button size="small" @click="emit('refresh')">Refresh</a-button>
    </div>
    <a-table :dataSource="trackedProcesses" :columns="columns" row-key="pid" size="small" :pagination="{pageSize:20}">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'cpu'"><span :style="{color: (record.cpu ?? 0) > 50 ? 'red' : 'inherit'}">{{ (record.cpu ?? 0).toFixed(1) }}%</span></template>
        <template v-if="column.key === 'action'">
          <a-space>
            <a-button type="link" size="small" @click="sendProcessSignal(record.pid, 'stop')">Suspend</a-button>
            <a-button type="link" size="small" @click="sendProcessSignal(record.pid, 'cont')">Resume</a-button>
            <a-button type="link" size="small" danger @click="sendProcessSignal(record.pid, 'kill')">Kill</a-button>
          </a-space>
        </template>
      </template>
    </a-table>
  </div>
</template>
