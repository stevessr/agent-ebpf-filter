<script setup lang="ts">
import { ref, computed } from 'vue';
import type { ProcessInfo } from '../../composables/useMonitorData';

const props = defineProps<{
  processes: ProcessInfo[];
  showProcessDetails: (record: any) => void;
  sendProcessSignal: (pid: number, signal: string) => void;
}>();

const processSearch = ref('');
const processViewMode = ref<'flat' | 'tree' | 'merged'>('flat');

const processColumns = [
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 100, sorter: (a: any, b: any) => a.pid - b.pid },
  { title: 'Name', dataIndex: 'name', key: 'name', sorter: (a: any, b: any) => a.name.localeCompare(b.name) },
  { title: 'CPU %', dataIndex: 'cpu', key: 'cpu', width: 90, align: 'right', sorter: (a: any, b: any) => a.cpu - b.cpu },
  { title: 'Mem %', dataIndex: 'mem', key: 'mem', width: 90, align: 'right', sorter: (a: any, b: any) => a.mem - b.mem },
  { title: 'GPU Util', dataIndex: 'gpuUtil', key: 'gpuUtil', width: 90, align: 'right', sorter: (a: any, b: any) => a.gpuUtil - b.gpuUtil },
  { title: 'VRAM', dataIndex: 'gpuMem', key: 'gpuMem', width: 90, align: 'right', sorter: (a: any, b: any) => a.gpuMem - b.gpuMem },
  { title: 'User', dataIndex: 'user', key: 'user', width: 100 },
  { title: 'Action', key: 'action', width: 260, align: 'right' },
];

const processedProcesses = computed(() => {
  let list = props.processes.map(p => ({ ...p, key: p.pid }));

  if (processSearch.value) {
    const q = processSearch.value.toLowerCase();
    list = list.filter(p => p.name.toLowerCase().includes(q) || p.pid.toString().includes(q) || (p.cmdline && p.cmdline.toLowerCase().includes(q)));
  }

  if (processViewMode.value === 'merged') {
    const merged: Record<string, any> = {};
    list.forEach(p => {
      if (!merged[p.name]) {
        merged[p.name] = { ...p, key: `group-${p.name}`, children: [], instances: 0, totalCpu: 0, totalMem: 0, totalGpuMem: 0, totalGpuUtil: 0 };
      }
      merged[p.name].instances++;
      merged[p.name].totalCpu += p.cpu;
      merged[p.name].totalMem += p.mem;
      merged[p.name].totalGpuMem += p.gpuMem;
      merged[p.name].totalGpuUtil += p.gpuUtil;
      merged[p.name].children.push({ ...p, key: p.pid });
    });
    return Object.values(merged).map(m => ({
      ...m,
      cpu: m.totalCpu,
      mem: m.totalMem,
      gpuMem: m.totalGpuMem,
      gpuUtil: m.totalGpuUtil,
      name: `${m.name} (${m.instances})`
    })).sort((a, b) => b.cpu - a.cpu);
  }

  if (processViewMode.value === 'tree') {
    const map: Record<number, any> = {};
    list.forEach(p => map[p.pid] = { ...p, key: p.pid, children: [] });
    const roots: any[] = [];
    list.forEach(p => {
      if (map[p.ppid] && p.ppid !== p.pid) {
        map[p.ppid].children.push(map[p.pid]);
      } else {
        roots.push(map[p.pid]);
      }
    });
    return roots;
  }

  return list;
});

const onShowDetails = (record: any) => {
  if (record.key && typeof record.key === 'string' && record.key.startsWith('group-')) return;
  props.showProcessDetails(record);
};

const onKill = (record: any) => {
  props.sendProcessSignal(record.pid, 'kill');
};
</script>

<template>
  <div style="background: #fff; padding: 20px; border-radius: 4px;">
    <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center; gap: 16px;">
      <a-space>
        <a-radio-group v-model:value="processViewMode" button-style="solid" size="small">
          <a-radio-button value="flat">Flat</a-radio-button>
          <a-radio-button value="tree">Tree</a-radio-button>
          <a-radio-button value="merged">Merged</a-radio-button>
        </a-radio-group>
        <a-input-search v-model:value="processSearch" placeholder="Search processes (name/PID/cmd)..." style="width: 300px" size="small" allow-clear />
      </a-space>
      <span style="font-size: 12px; color: #888;">Total: {{ processes.length }} processes</span>
    </div>
    <a-table :dataSource="processedProcesses" :columns="processColumns" size="small" rowKey="key" :scroll="{ y: 'calc(100vh - 420px)' }" :pagination="false">
      <template #bodyCell="{ column, record, text }">
        <template v-if="column.key === 'pid'">
          <span v-if="record.key && typeof record.key === 'string' && record.key.startsWith('group-')" style="color: #888;">Multiple</span>
          <span v-else style="font-family: monospace;">{{ text }}</span>
        </template>
        <template v-if="column.key === 'name'">
          <span style="font-weight: 500; cursor: pointer; color: #1890ff;" @click="onShowDetails(record)">{{ text }}</span>
        </template>
        <template v-if="column.key === 'cpu'">
          <span :style="{ color: (text ?? 0) > 50 ? '#ff4d4f' : 'inherit', fontWeight: (text ?? 0) > 20 ? 'bold' : 'normal' }">{{ (text ?? 0).toFixed(1) }}%</span>
        </template>
        <template v-if="column.key === 'mem'">
          <span>{{ (text ?? 0).toFixed(1) }}%</span>
        </template>
        <template v-if="column.key === 'gpuUtil'">
          <span v-if="text > 0">{{ text }}%</span>
          <span v-else style="color: #ccc;">-</span>
        </template>
        <template v-if="column.key === 'gpuMem'">
          <span v-if="text > 0">{{ text }} MB</span>
          <span v-else style="color: #ccc;">-</span>
        </template>
        <template v-if="column.key === 'action'">
          <a-space v-if="!(record.key && typeof record.key === 'string' && record.key.startsWith('group-'))">
            <a-button type="link" size="small" @click="onShowDetails(record)">Details</a-button>
            <a-button type="link" size="small" danger @click="onKill(record)">Kill</a-button>
          </a-space>
        </template>
      </template>
    </a-table>
  </div>
</template>
