<script setup lang="ts">
import type { FaultInfo } from '../../composables/useMonitorData';

defineProps<{
  faults: FaultInfo;
  topFaultProcesses: { pid: string | number; name: string; minorFaults: number; majorFaults: number; count?: number }[];
  faultTopN: number;
  mergeFaultProcesses: boolean;
  statsHistory: { faults: { time: number; value: number }[]; swapIn: { time: number; value: number }[]; swapOut: { time: number; value: number }[] };
  openHistoryChart: (title: string, datasets: { name: string; data: { time: number; value: number }[]; color?: string }[]) => void;
}>();

const emit = defineEmits<{
  'update:faultTopN': [value: number];
  'update:mergeFaultProcesses': [value: boolean];
}>();

</script>

<template>
  <a-row :gutter="16" style="padding-top: 16px;">
    <a-col :span="24">
      <a-card title="System Page Faults" size="small" :bordered="false" style="background: #fafafa;">
        <template #extra>
          <a-button type="link" size="small" @click="openHistoryChart('Page Fault Rate History', [{ name: 'Faults/s', data: statsHistory.faults, color: '#ff4d4f' }])">History Chart</a-button>
        </template>
        <a-row :gutter="16">
          <a-col :span="6"><a-statistic title="Soft Faults (Minor)" :value="faults.minorFaultRate" :precision="1" suffix="/s" @click="openHistoryChart('Minor Fault History', [{ name: 'Faults/s', data: statsHistory.faults, color: '#52c41a' }])" style="cursor: pointer;" /></a-col>
          <a-col :span="6"><a-statistic title="Hard Faults (Major)" :value="faults.majorFaultRate" :precision="1" suffix="/s" @click="openHistoryChart('Major Fault History', [{ name: 'Faults/s', data: statsHistory.faults, color: '#ff4d4f' }])" style="cursor: pointer;" /></a-col>
          <a-col :span="6"><a-statistic title="Swap-Out Rate" :value="faults.swapOutRate" :precision="1" suffix="/s" @click="openHistoryChart('Swap-Out Rate History', [{ name: 'Faults/s', data: statsHistory.swapOut, color: '#722ed1' }])" style="cursor: pointer;" /></a-col>
          <a-col :span="6"><a-statistic title="Swap-In Rate" :value="faults.swapInRate" :precision="1" suffix="/s" @click="openHistoryChart('Swap-In Rate History', [{ name: 'Faults/s', data: statsHistory.swapIn, color: '#13c2c2' }])" style="cursor: pointer;" /></a-col>
        </a-row>
      </a-card>
    </a-col>
    <a-col :span="24" style="margin-top: 16px;">
      <a-card title="Top Processes by Faults" size="small" :bordered="false" style="background: #fafafa;">
        <template #extra>
          <a-space>
            <span style="font-size: 12px; color: #888;">Top:</span>
            <a-select :value="faultTopN" size="small" style="width: 70px;" @change="emit('update:faultTopN', $event)">
              <a-select-option :value="5">5</a-select-option>
              <a-select-option :value="10">10</a-select-option>
              <a-select-option :value="20">20</a-select-option>
            </a-select>
            <a-checkbox :checked="mergeFaultProcesses" @change="emit('update:mergeFaultProcesses', ($event.target as any).checked)" style="font-size: 12px; color: #888;">Merge by Name</a-checkbox>
          </a-space>
        </template>
        <a-table :dataSource="topFaultProcesses" :columns="[
          { title: mergeFaultProcesses ? 'Group' : 'PID', dataIndex: mergeFaultProcesses ? 'name' : 'pid', key: 'id', width: mergeFaultProcesses ? 200 : 80 },
          { title: mergeFaultProcesses ? 'Instances' : 'Command', dataIndex: mergeFaultProcesses ? 'count' : 'name', key: 'name' },
          { title: 'Minor Faults', dataIndex: 'minorFaults', key: 'minorFaults', align: 'right' },
          { title: 'Major Faults', dataIndex: 'majorFaults', key: 'majorFaults', align: 'right' }
        ]" size="small" :pagination="false" rowKey="pid" />
      </a-card>
    </a-col>
  </a-row>
</template>
